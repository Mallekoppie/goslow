package platform

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type idTokenVerifierFunc func(context.Context, string) (map[string]interface{}, error)

func (f idTokenVerifierFunc) Verify(ctx context.Context, rawToken string) (map[string]interface{}, error) {
	return f(ctx, rawToken)
}

func TestHTTPLocalJWTRouteAuthorizesThroughTheIdentityAuthorizer(t *testing.T) {
	setupTestPlatformState(t)

	var (
		identity    Identity
		hasIdentity bool
	)
	router, err := newRouter(Routes{{
		Path:          "/admin",
		Method:        http.MethodGet,
		Auth:          AuthLocalJWT,
		RolesRequired: []string{"admin"},
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			identity, hasIdentity = IdentityFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	adminToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"sub":                "alice",
		"preferred_username": "alice",
		"roles":              []string{"admin"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected an authorized caller to reach the handler, got %d", response.Code)
	}
	if !hasIdentity {
		t.Fatal("expected the handler to read the caller through IdentityFromContext")
	}
	if identity.Username != "alice" {
		t.Fatalf("expected the verified username, got %q", identity.Username)
	}
	if len(identity.Roles) != 1 || identity.Roles[0] != "admin" {
		t.Fatalf("expected the verified roles, got %v", identity.Roles)
	}
	if identity.ClientToken != adminToken {
		t.Fatal("expected the bearer token to be retained for forwarding")
	}
}

func TestHTTPLocalJWTRouteRejectsCallersWithoutTheRequiredRole(t *testing.T) {
	setupTestPlatformState(t)

	handlerRan := false
	router, err := newRouter(Routes{{
		Path:          "/admin",
		Method:        http.MethodGet,
		Auth:          AuthLocalJWT,
		RolesRequired: []string{"admin"},
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			handlerRan = true
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	userToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"sub":   "bob",
		"roles": []string{"reader"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request.Header.Set("Authorization", "Bearer "+userToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected a verified caller without the role to receive 403, got %d", response.Code)
	}
	if handlerRan {
		t.Fatal("the handler must not run for a forbidden caller")
	}
}

func TestHTTPLocalJWTRouteRejectsMissingAndInvalidCredentials(t *testing.T) {
	setupTestPlatformState(t)

	router, err := newRouter(Routes{{
		Path:   "/private",
		Method: http.MethodGet,
		Auth:   AuthLocalJWT,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	testCases := []struct {
		name       string
		credential string
	}{
		{name: "missing credential"},
		{name: "malformed credential", credential: "Basic dXNlcjpwYXNz"},
		{name: "invalid bearer credential", credential: "Bearer not-a-token"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/private", nil)
			if testCase.credential != "" {
				request.Header.Set("Authorization", testCase.credential)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", response.Code)
			}
		})
	}
}

func TestHTTPOAuthRouteAuthorizationOutcomes(t *testing.T) {
	cfg := setupTestPlatformState(t)
	enableOAuth(t, cfg, "roles")
	stubOIDCVerifier(t, idTokenVerifierFunc(func(_ context.Context, rawToken string) (map[string]interface{}, error) {
		switch rawToken {
		case "admin-token":
			return map[string]interface{}{
				"iss":                "https://idp.example",
				"preferred_username": "alice",
				"roles":              []interface{}{"admin"},
			}, nil
		case "reader-token":
			return map[string]interface{}{
				"iss":   "https://idp.example",
				"roles": []interface{}{"reader"},
			}, nil
		default:
			return nil, errors.New("invalid token")
		}
	}), nil)

	var identity Identity
	var hasIdentity bool
	router, err := newRouter(Routes{{
		Path:          "/admin",
		Method:        http.MethodGet,
		Auth:          AuthOAuth,
		RolesRequired: []string{"admin"},
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			identity, hasIdentity = IdentityFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	testCases := []struct {
		name       string
		credential string
		wantStatus int
	}{
		{name: "verified caller holding the role", credential: "Bearer admin-token", wantStatus: http.StatusNoContent},
		{name: "verified caller without the role", credential: "Bearer reader-token", wantStatus: http.StatusForbidden},
		{name: "missing credential", wantStatus: http.StatusUnauthorized},
		{name: "rejected credential", credential: "Bearer invalid-token", wantStatus: http.StatusUnauthorized},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/admin", nil)
			if testCase.credential != "" {
				request.Header.Set("Authorization", testCase.credential)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != testCase.wantStatus {
				t.Fatalf("expected %d, got %d", testCase.wantStatus, response.Code)
			}
		})
	}

	if !hasIdentity {
		t.Fatal("expected the authorized handler to read the identity")
	}
	if identity.Username != "alice" || identity.Issuer != "https://idp.example" {
		t.Fatalf("expected the OAuth identity, got %q / %q", identity.Username, identity.Issuer)
	}
	if len(identity.Roles) != 1 || identity.Roles[0] != "admin" {
		t.Fatalf("expected the verified roles, got %v", identity.Roles)
	}
}

func TestHTTPOAuthRouteReportsAnIdentityProviderOutage(t *testing.T) {
	cfg := setupTestPlatformState(t)
	enableOAuth(t, cfg, "roles")
	stubOIDCVerifier(t, nil, errors.New("idp down"))

	handlerRan := false
	router, err := newRouter(Routes{{
		Path:   "/secure",
		Method: http.MethodGet,
		Auth:   AuthOAuth,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			handlerRan = true
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/secure", nil)
	request.Header.Set("Authorization", "Bearer token-value")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when the identity provider is unavailable, got %d", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "10" {
		t.Fatalf("expected a Retry-After header of 10 on the 503 response, got %q", got)
	}
	if handlerRan {
		t.Fatal("the handler must not run during an identity provider outage")
	}
}

func TestHTTPLocalOrOIDCRouteEnforcesRolesForBothStrategies(t *testing.T) {
	cfg := setupTestPlatformState(t)
	enableOAuth(t, cfg, "roles")
	stubOIDCVerifier(t, idTokenVerifierFunc(func(_ context.Context, rawToken string) (map[string]interface{}, error) {
		switch rawToken {
		case "oidc-admin":
			return map[string]interface{}{"roles": []interface{}{"admin"}}, nil
		case "oidc-reader":
			return map[string]interface{}{"roles": []interface{}{"reader"}}, nil
		default:
			return nil, errors.New("invalid token")
		}
	}), nil)

	router, err := newRouter(Routes{{
		Path:          "/either",
		Method:        http.MethodGet,
		Auth:          AuthLocalOrOIDC,
		RolesRequired: []string{"admin"},
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	localAdmin, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"roles": []string{"admin"}})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	localReader, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"roles": []string{"reader"}})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	testCases := []struct {
		name       string
		credential string
		wantStatus int
	}{
		{name: "local jwt holding the role", credential: "Bearer " + localAdmin, wantStatus: http.StatusNoContent},
		{name: "id token holding the role", credential: "Bearer oidc-admin", wantStatus: http.StatusNoContent},
		{name: "local jwt without the role", credential: "Bearer " + localReader, wantStatus: http.StatusForbidden},
		{name: "id token without the role", credential: "Bearer oidc-reader", wantStatus: http.StatusForbidden},
		{name: "no credential", wantStatus: http.StatusUnauthorized},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/either", nil)
			if testCase.credential != "" {
				request.Header.Set("Authorization", testCase.credential)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != testCase.wantStatus {
				t.Fatalf("expected %d, got %d", testCase.wantStatus, response.Code)
			}
		})
	}
}

func TestHTTPPublicRoutePerformsNoCredentialWork(t *testing.T) {
	cfg := setupTestPlatformState(t)
	enableOAuth(t, cfg, "roles")

	verifierCalls := 0
	original := newOIDCClaimsVerifier
	t.Cleanup(func() { newOIDCClaimsVerifier = original })
	newOIDCClaimsVerifier = func(context.Context, OAuthSettings, idTokenAudiencePolicy) (IDTokenVerifier, error) {
		verifierCalls++
		return idTokenVerifierFunc(func(context.Context, string) (map[string]interface{}, error) {
			t.Fatal("a public route must not verify credentials")
			return nil, nil
		}), nil
	}

	var hasIdentity bool
	router, err := newRouter(Routes{{
		Path:   "/public",
		Method: http.MethodGet,
		Auth:   AuthNone,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			_, hasIdentity = IdentityFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/public", nil)
	request.Header.Set("Authorization", "Bearer should-not-be-read")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected the public route to run, got %d", response.Code)
	}
	if verifierCalls != 0 {
		t.Fatalf("a public route must not contact the identity provider, factory called %d times", verifierCalls)
	}
	if hasIdentity {
		t.Fatal("a public route must not store an identity")
	}
}

func TestHTTPRouteCapturesItsRuleAndSettingsAtConstruction(t *testing.T) {
	cfg := setupTestPlatformState(t)

	routes := Routes{{
		Path:          "/private",
		Method:        http.MethodGet,
		Auth:          AuthLocalJWT,
		RolesRequired: []string{"admin"},
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}}
	router, err := newRouter(routes)
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	// Changing the caller's route value after construction must not change the
	// resolved rule.
	routes[0].Auth = AuthNone
	routes[0].RolesRequired = nil

	builtToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"roles": []string{"admin"}})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	changed := *cfg
	changed.Auth.Server.LocalJwt.JwtSigningKey = "changed-signing-key"
	if err := SetPlatformConfiguration(changed); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}
	changedToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"roles": []string{"admin"}})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	// No credential still requires authentication, proving the original rule and
	// not the mutated AuthNone is in force.
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/private", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected the captured rule to stay in force, got %d", response.Code)
	}

	// The original settings snapshot still verifies tokens signed with the old key.
	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	request.Header.Set("Authorization", "Bearer "+builtToken)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected the original signing key to verify, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/private", nil)
	request.Header.Set("Authorization", "Bearer "+changedToken)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected the changed signing key to be ignored, got %d", response.Code)
	}
}

func TestHTTPAuthFailuresAreAccessLogged(t *testing.T) {
	setupTestPlatformState(t)

	var buffer bytes.Buffer
	previous := Log.current.Load()
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&buffer), zap.InfoLevel)
	Log.current.Store(zap.New(core))
	t.Cleanup(func() { Log.current.Store(previous) })

	router, err := newRouter(Routes{{
		Path:          "/admin",
		Method:        http.MethodGet,
		Auth:          AuthLocalJWT,
		RolesRequired: []string{"admin"},
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"roles": []string{"reader"}})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
	logged := buffer.String()
	if !strings.Contains(logged, `"msg":"Request completed"`) {
		t.Fatalf("expected the completed request to be access logged, got:\n%s", logged)
	}
	if !strings.Contains(logged, `"statuscode":403`) {
		t.Fatalf("expected the forbidden status to be access logged, got:\n%s", logged)
	}
	if strings.Contains(logged, token) {
		t.Fatal("access logging must not record the bearer token")
	}
}

func TestHTTPOAuthRouteReportsAnOutageWhenProviderKeysCannotBeFetched(t *testing.T) {
	cfg := setupTestPlatformState(t)
	clearOIDCProviderCache()
	idp := newFakeIDP(t)

	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = idp.server.URL
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	cfg.Auth.Server.OAuth.AllowedAlgorithms = []string{"RS256"}
	cfg.Auth.Server.OAuth.RolesClaim = "roles"
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

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

	// The first request discovers the provider and caches its signing keys.
	firstToken := idp.signToken(t, idp.validClaims())
	firstRequest := httptest.NewRequest(http.MethodGet, "/secure", nil)
	firstRequest.Header.Set("Authorization", "Bearer "+firstToken)
	firstResponse := httptest.NewRecorder()
	router.ServeHTTP(firstResponse, firstRequest)
	if firstResponse.Code != http.StatusNoContent {
		t.Fatalf("expected the first token to be accepted, got %d", firstResponse.Code)
	}

	// A token with an unknown key id forces a JWKS fetch. With the provider
	// unreachable, that is an outage, not a rejected credential.
	idp.server.Close()
	rotated := idp.validClaims()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, rotated)
	token.Header["kid"] = "rotated-key"
	rotatedToken, err := token.SignedString(idp.key)
	if err != nil {
		t.Fatalf("sign rotated token: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/secure", nil)
	request.Header.Set("Authorization", "Bearer "+rotatedToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when signing keys cannot be fetched, got %d", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "10" {
		t.Fatalf("expected a Retry-After header of 10 on the 503 response, got %q", got)
	}
}
