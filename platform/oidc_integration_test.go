package platform

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc"
	jwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type fakeIDP struct {
	server        *httptest.Server
	key           *rsa.PrivateKey
	keyID         string
	discoveryHits int32
	jwksHits      int32
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
		publicKey := key.PublicKey
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": idp.keyID,
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

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = f.keyID
	signed, err := token.SignedString(f.key)
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

func TestOAuthVerifierAgainstRealOIDCDiscovery(t *testing.T) {
	cfg := setupTestPlatformState(t)
	idp := newFakeIDP(t)

	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = idp.server.URL
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	cfg.Auth.Server.OAuth.AllowedAlgorithms = []string{"rs256"}
	cfg.Auth.Server.OAuth.RolesClaim = "roles"
	SetPlatformConfiguration(*cfg)

	rawToken := idp.signToken(t, idp.validClaims())
	request := httptest.NewRequest(http.MethodGet, "/secure", nil)
	request.Header.Set("Authorization", "Bearer "+rawToken)

	result, err := oauthVerifier([]string{"admin"})(request)
	if err != nil {
		t.Fatalf("oauthVerifier failed: %v", err)
	}
	if len(result.Roles) != 1 || result.Roles[0] != "admin" {
		t.Fatalf("expected the admin role, got %v", result.Roles)
	}
	if result.Issuer != idp.server.URL || result.Username != "alice" {
		t.Fatalf("unexpected result: %+v", result)
	}

	// A second verification must reuse the cached provider.
	if _, err := oauthVerifier(nil)(request); err != nil {
		t.Fatalf("second verification failed: %v", err)
	}
	if got := atomic.LoadInt32(&idp.discoveryHits); got != 1 {
		t.Fatalf("expected one discovery call, got %d", got)
	}
}

func TestOAuthVerifierRejectsWrongAudienceAndExpiredTokens(t *testing.T) {
	cfg := setupTestPlatformState(t)
	idp := newFakeIDP(t)

	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = idp.server.URL
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	SetPlatformConfiguration(*cfg)

	claims := idp.validClaims()
	claims["aud"] = "someone-else"
	request := httptest.NewRequest(http.MethodGet, "/secure", nil)
	request.Header.Set("Authorization", "Bearer "+idp.signToken(t, claims))
	if _, err := oauthVerifier(nil)(request); err == nil {
		t.Fatal("expected the wrong audience to be rejected")
	}

	claims = idp.validClaims()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	request = httptest.NewRequest(http.MethodGet, "/secure", nil)
	request.Header.Set("Authorization", "Bearer "+idp.signToken(t, claims))
	if _, err := oauthVerifier(nil)(request); err == nil {
		t.Fatal("expected an expired token to be rejected")
	}
}

func TestOAuthVerifierReportsUnreachableProvider(t *testing.T) {
	cfg := setupTestPlatformState(t)

	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = "http://127.0.0.1:1/.well-known/openid-configuration"
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	SetPlatformConfiguration(*cfg)

	request := httptest.NewRequest(http.MethodGet, "/secure", nil)
	request.Header.Set("Authorization", "Bearer token")
	_, err := oauthVerifier(nil)(request)
	if !errors.Is(err, errOIDCUnavailable) {
		t.Fatalf("expected errOIDCUnavailable, got %v", err)
	}
}

func TestValidateClientTokenWithFakeIDP(t *testing.T) {
	resetPlatformState(t)
	idp := newFakeIDP(t)

	rawToken := idp.signToken(t, idp.validClaims())

	parsed, claims, err := OAuth.ValidateClientToken(rawToken)
	if err != nil {
		t.Fatalf("ValidateClientToken failed: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("expected a valid token")
	}
	if claims["preferred_username"] != "alice" {
		t.Fatalf("unexpected claims: %v", claims)
	}

	// A second call should reuse the cached discovery document and JWKS.
	if _, _, err := OAuth.ValidateClientToken(rawToken); err != nil {
		t.Fatalf("second ValidateClientToken failed: %v", err)
	}
	if got := atomic.LoadInt32(&idp.discoveryHits); got != 1 {
		t.Fatalf("expected one discovery call, got %d", got)
	}
	if got := atomic.LoadInt32(&idp.jwksHits); got != 1 {
		t.Fatalf("expected one JWKS fetch, got %d", got)
	}
}

func TestValidateClientTokenRejectsMalformedInput(t *testing.T) {
	resetPlatformState(t)

	if _, _, err := OAuth.ValidateClientToken("not-a-token"); err == nil {
		t.Fatal("expected an error for a malformed token")
	}

	noIssuer := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "alice"})
	signed, err := noIssuer.SignedString([]byte("key"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	if _, _, err := OAuth.ValidateClientToken(signed); err == nil {
		t.Fatal("expected an error for a token without an issuer")
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

func TestLegacyMiddlewareWrappers(t *testing.T) {
	cfg := setupTestPlatformState(t)
	hashBytes, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	hash := string(hashBytes)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// basicAuthMiddleware
	basic := basicAuthMiddleware(okHandler, map[string]string{"alice": hash})
	request := httptest.NewRequest(http.MethodGet, "/basic", nil)
	request.SetBasicAuth("alice", "s3cret")
	response := httptest.NewRecorder()
	basic.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected basic auth to pass, got %d", response.Code)
	}

	// oAuth2Middleware with an injected verifier
	stubOIDCVerifier(t, fakeClaimsVerifier{claims: map[string]interface{}{"roles": []interface{}{"admin"}}}, nil)
	cfg.Auth.Server.OAuth.Enabled = true
	SetPlatformConfiguration(*cfg)

	oauth := oAuth2Middleware(okHandler, []string{"admin"})
	request = httptest.NewRequest(http.MethodGet, "/oauth", nil)
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	oauth.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected oauth middleware to pass, got %d", response.Code)
	}

	// localJwtAuthMiddleware
	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "alice"})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	local := localJwtAuthMiddleware(okHandler)
	request = httptest.NewRequest(http.MethodGet, "/local", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	local.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected local jwt middleware to pass, got %d", response.Code)
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
