package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGRPCAuthInterceptor(t *testing.T) {
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "grpc-test-key"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	cfg.Grpc.Server.UnAuthenticatedPaths = []string{"/public.Service/Call"}
	SetPlatformConfiguration(*cfg)

	called := false
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return "ok", nil
	}

	// Unauthenticated paths skip token checks.
	response, err := grpcAuthInterceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/public.Service/Call"}, handler)
	if err != nil || response != "ok" || !called {
		t.Fatalf("expected the public path to call the handler, got %v / %v", response, err)
	}

	// Missing metadata is rejected.
	called = false
	_, err = grpcAuthInterceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/private.Service/Call"}, handler)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for missing metadata, got %v", err)
	}
	if called {
		t.Fatal("handler must not run without metadata")
	}

	// Missing authorization is rejected.
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("other", "value"))
	_, err = grpcAuthInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/private.Service/Call"}, handler)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated for missing authorization, got %v", err)
	}

	// Malformed authorization is rejected.
	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Basic abc"))
	_, err = grpcAuthInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/private.Service/Call"}, handler)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated for a non-bearer token, got %v", err)
	}

	// A valid local JWT is accepted and its claims reach the handler.
	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "alice"})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))

	var claims map[string]interface{}
	handler = func(ctx context.Context, req interface{}) (interface{}, error) {
		claims, _ = ctx.Value(ContextLocalJwtClaims).(map[string]interface{})
		return "ok", nil
	}
	if _, err := grpcAuthInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/private.Service/Call"}, handler); err != nil {
		t.Fatalf("expected a valid token to pass, got %v", err)
	}
	if claims["sub"] != "alice" {
		t.Fatalf("expected claims in the context, got %v", claims)
	}
}

func TestInitializeVaultWithConfiguredAddress(t *testing.T) {
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Vault.Enabled = true
	cfg.Vault.AddressList = []string{"http://127.0.0.1:8200"}
	cfg.Vault.TimeoutSeconds = 1
	cfg.Vault.Token.Enabled = true
	cfg.Vault.Token.Token = "vault-token"
	SetPlatformConfiguration(*cfg)

	initializeVault()

	if !vaultEnabled || !vaultInitialized {
		t.Fatalf("expected Vault to be initialized, enabled=%v initialized=%v", vaultEnabled, vaultInitialized)
	}
	if len(vaultClientList) != 1 {
		t.Fatalf("expected one Vault client, got %d", len(vaultClientList))
	}

	// A second call must not rebuild the clients.
	initializeVault()
	if len(vaultClientList) != 1 {
		t.Fatalf("expected the Vault clients to stay stable, got %d", len(vaultClientList))
	}
}

func TestOAuthTokenRenewalLoopPopulatesCache(t *testing.T) {
	resetPlatformState(t)

	signedToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "client",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("idp-key"))
	if err != nil {
		t.Fatalf("sign idp token: %v", err)
	}

	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "password" {
			t.Errorf("expected the password grant, got %q", r.Form.Get("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": signedToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer idp.Close()

	cfg := createDefaultConfiguration()
	cfg.Auth.Client.OAuth = []clientTokenConfig{{
		ID:                        "demo",
		IdpTokenEndpoint:          idp.URL,
		ClientID:                  "client",
		ClientSecret:              "secret",
		Username:                  "user",
		Password:                  "pass",
		RenewCheckIntervalSeconds: 0.01,
		RenewCheckTimeMinutes:     1,
	}}
	SetPlatformConfiguration(*cfg)

	initializeOAuthTokenClients()
	t.Cleanup(stopOAuthTokenRenewal)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		oAuthStateLock.RLock()
		token := oAuthTokens["demo"]
		oAuthStateLock.RUnlock()
		if token != "" {
			if token != signedToken {
				t.Fatalf("expected the IDP token in the cache")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("token cache was never populated")
}

func TestInternalGetOAuth2TokenReportsIDPErrors(t *testing.T) {
	resetPlatformState(t)

	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer idp.Close()

	cfg := createDefaultConfiguration()
	cfg.Auth.Client.OAuth = []clientTokenConfig{{
		ID:               "broken",
		IdpTokenEndpoint: idp.URL,
		ClientID:         "client",
		ClientSecret:     "secret",
	}}
	SetPlatformConfiguration(*cfg)
	oAuthStateLock.Lock()
	oAuthClientTokenConfiguration = cfg.Auth.Client.OAuth
	oAuthHttpClient = createOAuthHTTPClient()
	oAuthStateLock.Unlock()

	if _, err := internalGetOAuth2Token("broken"); err != ErrOAuthIncorrectIDPStatusCode {
		t.Fatalf("expected ErrOAuthIncorrectIDPStatusCode, got %v", err)
	}
	if _, err := internalGetOAuth2Token("missing"); err != ErrOAuthClientConfigNotFound {
		t.Fatalf("expected ErrOAuthClientConfigNotFound, got %v", err)
	}
}
