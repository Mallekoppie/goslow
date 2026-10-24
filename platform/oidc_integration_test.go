package platform

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

type fakeIDP struct {
	mu              sync.RWMutex
	server          *httptest.Server
	key             *rsa.PrivateKey
	keyID           string
	discoveryHits   int32
	jwksHits        int32
	jwksStatus      atomic.Int32
	keyFetchStarted chan struct{}
	keyFetchRelease chan struct{}
}

// rotateKey replaces the signing key and changes its ID, as a real provider
// does when it rotates keys.
func (f *fakeIDP) rotateKey(t *testing.T) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rotated rsa key: %v", err)
	}

	f.mu.Lock()
	f.key = key
	f.keyID = "rotated-key"
	f.mu.Unlock()
}

func (f *fakeIDP) signingKey() (*rsa.PrivateKey, string) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.key, f.keyID
}

func (f *fakeIDP) failJWKS(status int) {
	f.jwksStatus.Store(int32(status))
}

// blockKeyFetch holds the next JWKS response open until release is called. The
// started channel closes once the handler has received the request, so a test
// can cancel the caller while the key fetch is genuinely in flight.
func (f *fakeIDP) blockKeyFetch() (started <-chan struct{}, release func()) {
	startedCh := make(chan struct{})
	releaseCh := make(chan struct{})

	f.mu.Lock()
	f.keyFetchStarted = startedCh
	f.keyFetchRelease = releaseCh
	f.mu.Unlock()

	var once sync.Once
	return startedCh, func() { once.Do(func() { close(releaseCh) }) }
}

func clearOIDCProviderCache() {
	oidcProviderLock.Lock()
	oidcProviderCache = make(map[string]*oidc.Provider)
	oidcProviderLock.Unlock()
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	idp := &fakeIDP{key: key, keyID: "test-key"}
	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&idp.discoveryHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"issuer":                                idp.server.URL,
			"jwks_uri":                              idp.server.URL + "/keys",
			"authorization_endpoint":                idp.server.URL + "/auth",
			"token_endpoint":                        idp.server.URL + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})

	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&idp.jwksHits, 1)
		if status := idp.jwksStatus.Load(); status != 0 {
			http.Error(w, "jwks unavailable", int(status))
			return
		}

		idp.mu.Lock()
		started, release := idp.keyFetchStarted, idp.keyFetchRelease
		idp.keyFetchStarted, idp.keyFetchRelease = nil, nil
		idp.mu.Unlock()
		if release != nil {
			if started != nil {
				close(started)
			}
			<-release
		}

		currentKey, currentKeyID := idp.signingKey()
		publicKey := currentKey.PublicKey
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": currentKeyID,
				"use": "sig",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
			}},
		})
	})

	idp.server = httptest.NewServer(mux)
	t.Cleanup(func() {
		idp.server.Close()
		clearOIDCProviderCache()
	})
	return idp
}

func (f *fakeIDP) signToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	key, keyID := f.signingKey()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func (f *fakeIDP) validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":                f.server.URL,
		"aud":                "test-client",
		"sub":                "user-1",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
		"preferred_username": "alice",
		"roles":              []string{"admin"},
	}
}

func TestOIDCClaimsVerifierAgainstRealOIDCDiscovery(t *testing.T) {
	cfg := setupTestPlatformState(t)
	idp := newFakeIDP(t)

	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = idp.server.URL
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	cfg.Auth.Server.OAuth.AllowedAlgorithms = []string{"rs256"}
	cfg.Auth.Server.OAuth.RolesClaim = "roles"
	SetPlatformConfiguration(*cfg)

	verifier, err := newOIDCClaimsVerifier(context.Background(), AuthSettingsFromConfig(cfg).OAuth, requireConfiguredClientID)
	if err != nil {
		t.Fatalf("newOIDCClaimsVerifier failed: %v", err)
	}

	rawToken := idp.signToken(t, idp.validClaims())
	claims, err := verifier.Verify(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	roles, _ := rolesFromClaim(claims, cfg.Auth.Server.OAuth.RolesClaim)
	if len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("expected the admin role, got %v", roles)
	}
	if claims["iss"] != idp.server.URL || claims["preferred_username"] != "alice" {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	// A second verification must reuse the cached provider.
	if _, err := verifier.Verify(context.Background(), rawToken); err != nil {
		t.Fatalf("second verification failed: %v", err)
	}
	if got := atomic.LoadInt32(&idp.discoveryHits); got != 1 {
		t.Fatalf("expected one discovery call, got %d", got)
	}
}

func TestOIDCClaimsVerifierRejectsWrongAudienceAndExpiredTokens(t *testing.T) {
	cfg := setupTestPlatformState(t)
	idp := newFakeIDP(t)

	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = idp.server.URL
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	SetPlatformConfiguration(*cfg)

	verifier, err := newOIDCClaimsVerifier(context.Background(), AuthSettingsFromConfig(cfg).OAuth, requireConfiguredClientID)
	if err != nil {
		t.Fatalf("newOIDCClaimsVerifier failed: %v", err)
	}

	claims := idp.validClaims()
	claims["aud"] = "someone-else"
	if _, err := verifier.Verify(context.Background(), idp.signToken(t, claims)); err == nil {
		t.Fatal("expected the wrong audience to be rejected")
	}

	claims = idp.validClaims()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	if _, err := verifier.Verify(context.Background(), idp.signToken(t, claims)); err == nil {
		t.Fatal("expected an expired token to be rejected")
	}
}

func TestAuthorizerReportsUnreachableProvider(t *testing.T) {
	cfg := setupTestPlatformState(t)

	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = "http://127.0.0.1:1"
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	SetPlatformConfiguration(*cfg)

	authorizer, err := newAuthorizerFromConfig(cfg)
	if err != nil {
		t.Fatalf("newAuthorizerFromConfig failed: %v", err)
	}
	rule, err := authorizer.ResolveAccessRule(AuthOAuth, nil)
	if err != nil {
		t.Fatalf("ResolveAccessRule failed: %v", err)
	}

	if _, err := authorizer.Authorize(context.Background(), rule, BearerCredential("token")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestNormalizeAlgorithms(t *testing.T) {
	got := normalizeAlgorithms([]string{" rs256 ", "RS384", "es256"})
	want := []string{"RS256", "RS384", "ES256"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestAllowedContentTypeMiddleware(t *testing.T) {
	setupTestPlatformState(t)

	router, err := newRouter(Routes{{
		Path:               "/json",
		Method:             http.MethodPost,
		Auth:               AuthNone,
		AllowedContentType: "application/json",
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/json", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for the allowed content type, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/json", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "text/plain")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for a disallowed content type, got %d", response.Code)
	}
}

func TestAuthStrategyEnabledAndLoggerWarn(t *testing.T) {
	if !AuthOAuth.Enabled() || !AuthLocalOrOIDC.Enabled() {
		t.Fatal("expected auth strategies to be enabled")
	}
	if AuthNone.Enabled() || AuthStrategy("").Enabled() {
		t.Fatal("expected none/empty strategies to be disabled")
	}

	resetPlatformState(t)
	Log.Warn("warn level is exercised", zap.String("test", "true"))
}
