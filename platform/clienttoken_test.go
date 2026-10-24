package platform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// configureOAuthForClientTokens points the platform at the fake identity
// provider. The configured client ID belongs to this service, not to the
// client that minted a forwarded token.
func configureOAuthForClientTokens(t *testing.T, idp *fakeIDP) *Config {
	t.Helper()

	cfg := setupTestPlatformState(t)
	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = idp.server.URL
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	cfg.Auth.Server.OAuth.AllowedAlgorithms = []string{"RS256"}
	SetPlatformConfiguration(*cfg)
	return cfg
}

func TestClientTokenAudiencePolicyIsExplicit(t *testing.T) {
	idp := newFakeIDP(t)
	configureOAuthForClientTokens(t, idp)

	claims := idp.validClaims()
	claims["aud"] = "a-different-client"
	rawToken := idp.signToken(t, claims)

	// Route authorization proves this service's own caller, so the token must
	// have been issued to the configured client.
	router, err := newRouter(Routes{{
		Path:   "/secure",
		Method: http.MethodGet,
		Auth:   AuthOAuth,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/secure", nil)
	request.Header.Set("Authorization", "Bearer "+rawToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("route authorization must reject a token issued to another client, got %d", response.Code)
	}

	// A forwarded client token is evidence about a different client, so it is
	// verified without an audience check.
	verifiedClaims, err := OAuth.ValidateClientToken(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("a forwarded client token must not be audience-bound to this client: %v", err)
	}
	if verifiedClaims["preferred_username"] != "alice" {
		t.Fatalf("unexpected forwarded token claims: %v", verifiedClaims)
	}
}

func TestValidateClientTokenVerificationOutcomes(t *testing.T) {
	idp := newFakeIDP(t)
	configureOAuthForClientTokens(t, idp)

	expiredClaims := idp.validClaims()
	expiredClaims["exp"] = time.Now().Add(-time.Hour).Unix()

	wrongIssuerClaims := idp.validClaims()
	wrongIssuerClaims["iss"] = "https://a-different-issuer.example"

	testCases := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{name: "valid", token: idp.signToken(t, idp.validClaims())},
		{name: "expired", token: idp.signToken(t, expiredClaims), wantErr: true},
		{name: "wrong issuer", token: idp.signToken(t, wrongIssuerClaims), wantErr: true},
		{name: "malformed", token: "not-a-token", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			claims, err := OAuth.ValidateClientToken(context.Background(), testCase.token)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected verification to fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected verification to succeed: %v", err)
			}
			if claims["preferred_username"] != "alice" {
				t.Fatalf("unexpected claims: %v", claims)
			}
		})
	}
}

func TestValidateClientTokenReportsProviderUnavailable(t *testing.T) {
	cfg := setupTestPlatformState(t)
	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = "http://127.0.0.1:1"
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	SetPlatformConfiguration(*cfg)

	_, err := OAuth.ValidateClientToken(context.Background(), "opaque-token")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected an unavailable identity provider, got %v", err)
	}
}

func TestValidateClientTokenReportsUnavailableWhenKeysCannotBeFetched(t *testing.T) {
	idp := newFakeIDP(t)
	configureOAuthForClientTokens(t, idp)
	idp.failJWKS(http.StatusInternalServerError)

	_, err := OAuth.ValidateClientToken(context.Background(), idp.signToken(t, idp.validClaims()))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected a key-fetch failure to be reported as unavailable, got %v", err)
	}
}

func TestValidateClientTokenDoesNotMisreportMissingConfiguration(t *testing.T) {
	cfg := setupTestPlatformState(t)
	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	SetPlatformConfiguration(*cfg)

	_, err := OAuth.ValidateClientToken(context.Background(), "opaque-token")
	if err == nil {
		t.Fatal("expected missing configuration to fail")
	}
	if errors.Is(err, ErrUnavailable) {
		t.Fatalf("a local configuration error is not an identity-provider outage: %v", err)
	}
}

func TestValidateClientTokenHonoursContextCancellation(t *testing.T) {
	idp := newFakeIDP(t)
	configureOAuthForClientTokens(t, idp)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := OAuth.ValidateClientToken(ctx, idp.signToken(t, idp.validClaims()))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation to stop verification, got %v", err)
	}
}

func TestValidateClientTokenHonoursCancellationDuringKeyFetch(t *testing.T) {
	idp := newFakeIDP(t)
	configureOAuthForClientTokens(t, idp)
	started, release := idp.blockKeyFetch()
	defer release()

	rawToken := idp.signToken(t, idp.validClaims())
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := OAuth.ValidateClientToken(ctx, rawToken)
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the identity provider key fetch did not start")
	}
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation to stop the key fetch, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not stop verification")
	}
}

func TestValidateClientTokenRefreshesKeysForUnknownKeyID(t *testing.T) {
	idp := newFakeIDP(t)
	configureOAuthForClientTokens(t, idp)

	firstToken := idp.signToken(t, idp.validClaims())
	if _, err := OAuth.ValidateClientToken(context.Background(), firstToken); err != nil {
		t.Fatalf("first verification failed: %v", err)
	}
	if got := atomic.LoadInt32(&idp.jwksHits); got != 1 {
		t.Fatalf("expected one JWKS fetch, got %d", got)
	}

	idp.rotateKey(t)

	rotatedToken := idp.signToken(t, idp.validClaims())
	if _, err := OAuth.ValidateClientToken(context.Background(), rotatedToken); err != nil {
		t.Fatalf("an unknown key id must refresh the provider keys: %v", err)
	}
	if got := atomic.LoadInt32(&idp.jwksHits); got != 2 {
		t.Fatalf("expected the JWKS to be fetched again, got %d fetches", got)
	}
}

func TestValidateClientTokenReusesCachedDiscoveryAndKeys(t *testing.T) {
	idp := newFakeIDP(t)
	configureOAuthForClientTokens(t, idp)

	rawToken := idp.signToken(t, idp.validClaims())
	for range 2 {
		if _, err := OAuth.ValidateClientToken(context.Background(), rawToken); err != nil {
			t.Fatalf("verification failed: %v", err)
		}
	}

	if got := atomic.LoadInt32(&idp.discoveryHits); got != 1 {
		t.Fatalf("expected one discovery call, got %d", got)
	}
	if got := atomic.LoadInt32(&idp.jwksHits); got != 1 {
		t.Fatalf("expected one JWKS fetch, got %d", got)
	}
}
