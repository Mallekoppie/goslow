package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

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
