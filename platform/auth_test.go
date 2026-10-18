package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

type fakeClaimsVerifier struct {
	claims map[string]interface{}
	err    error
}

func (f fakeClaimsVerifier) Verify(ctx context.Context, rawToken string) (map[string]interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

func stubOIDCVerifier(t *testing.T, verifier oidcClaimsVerifier, err error) {
	t.Helper()
	original := newOIDCClaimsVerifier
	t.Cleanup(func() { newOIDCClaimsVerifier = original })
	newOIDCClaimsVerifier = func(ctx context.Context, conf *Config) (oidcClaimsVerifier, error) {
		if err != nil {
			return nil, err
		}
		return verifier, nil
	}
}

func enableOAuth(t *testing.T, cfg *Config, rolesClaim string) {
	t.Helper()
	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = "https://idp.example/.well-known/openid-configuration"
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	cfg.Auth.Server.OAuth.RolesClaim = rolesClaim
	SetPlatformConfiguration(*cfg)
}

func TestExtractRoles(t *testing.T) {
	testCases := []struct {
		name      string
		claims    map[string]interface{}
		claimPath string
		want      []string
	}{
		{
			name:      "string list",
			claims:    map[string]interface{}{"roles": []interface{}{"reader", "writer"}},
			claimPath: "roles",
			want:      []string{"reader", "writer"},
		},
		{
			name:      "typed string list",
			claims:    map[string]interface{}{"roles": []string{"reader"}},
			claimPath: "roles",
			want:      []string{"reader"},
		},
		{
			name:      "object keyed roles",
			claims:    map[string]interface{}{"roles": map[string]interface{}{"writer": nil, "admin": nil}},
			claimPath: "roles",
			want:      []string{"admin", "writer"},
		},
		{
			name:      "dotted path",
			claims:    map[string]interface{}{"realm_access": map[string]interface{}{"roles": []interface{}{"admin"}}},
			claimPath: "realm_access.roles",
			want:      []string{"admin"},
		},
		{
			name:      "literal urn key wins over dotted traversal",
			claims:    map[string]interface{}{"urn:zitadel:iam:org:project:roles": map[string]interface{}{"owner": nil}},
			claimPath: "urn:zitadel:iam:org:project:roles",
			want:      []string{"owner"},
		},
		{
			name:      "missing claim",
			claims:    map[string]interface{}{},
			claimPath: "roles",
			want:      nil,
		},
		{
			name:      "empty path defaults to roles",
			claims:    map[string]interface{}{"roles": []interface{}{"admin"}},
			claimPath: "",
			want:      []string{"admin"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := extractRoles(testCase.claims, testCase.claimPath)
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestAllAuthMiddlewareRequiresEveryVerifier(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	allow := func(*http.Request) (authResult, error) { return authResult{}, nil }
	deny := func(*http.Request) (authResult, error) { return authResult{}, ErrAuthRequired }

	req := httptest.NewRequest(http.MethodGet, "/secure", nil)

	res := httptest.NewRecorder()
	allAuthMiddleware(handler, allow, allow).ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204 when every verifier passes, got %d", res.Code)
	}

	res = httptest.NewRecorder()
	allAuthMiddleware(handler, allow, deny).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when one verifier fails, got %d", res.Code)
	}
}

func TestAnyAuthMiddlewareAcceptsOneVerifier(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	deny := func(*http.Request) (authResult, error) { return authResult{}, ErrAuthRequired }
	allow := func(*http.Request) (authResult, error) { return authResult{Username: "alice"}, nil }

	res := httptest.NewRecorder()
	anyAuthMiddleware(handler, deny, allow).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/secure", nil))
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204 when one verifier passes, got %d", res.Code)
	}

	res = httptest.NewRecorder()
	anyAuthMiddleware(handler, deny, deny).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/secure", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when every verifier fails, got %d", res.Code)
	}
}

func TestAnyAuthMiddlewareReportsIDPOutage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run when the identity provider is unavailable")
	})

	unavailable := func(*http.Request) (authResult, error) {
		return authResult{}, errOIDCUnavailable
	}

	res := httptest.NewRecorder()
	anyAuthMiddleware(handler, unavailable).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/secure", nil))

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when the IdP is unavailable, got %d", res.Code)
	}
	if res.Header().Get("Retry-After") == "" {
		t.Fatal("expected a Retry-After header on the 503 response")
	}
}

func TestOAuthRouteWithoutRolesInvokesHandler(t *testing.T) {
	cfg := setupTestPlatformState(t)
	enableOAuth(t, cfg, "roles")
	stubOIDCVerifier(t, fakeClaimsVerifier{claims: map[string]interface{}{
		"iss":                "https://idp.example",
		"preferred_username": "alice",
	}}, nil)

	var issuer, username string
	router, err := newRouter(Routes{{
		Path:   "/open",
		Method: http.MethodGet,
		Auth:   AuthOAuth,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			issuer = IssuerFromContext(r.Context())
			username = UsernameFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/open", nil)
	req.Header.Set("Authorization", "Bearer token-value")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for a role-less protected route, got %d", res.Code)
	}
	if issuer != "https://idp.example" || username != "alice" {
		t.Fatalf("expected issuer and username in context, got %q / %q", issuer, username)
	}
}

func TestOAuthRouteEnforcesRoles(t *testing.T) {
	cfg := setupTestPlatformState(t)
	enableOAuth(t, cfg, "urn:zitadel:iam:org:project:roles")
	stubOIDCVerifier(t, fakeClaimsVerifier{claims: map[string]interface{}{
		"urn:zitadel:iam:org:project:roles": map[string]interface{}{"reader": nil},
	}}, nil)

	var granted []string
	router, err := newRouter(Routes{{
		Path:          "/admin",
		Method:        http.MethodGet,
		Auth:          AuthOAuth,
		RolesRequired: []string{"admin"},
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			granted = RolesFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer token-value")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when the required role is missing, got %d", res.Code)
	}
	if len(granted) != 0 {
		t.Fatalf("handler must not run, got roles %v", granted)
	}
}

func TestLocalOrOIDCAcceptsEitherToken(t *testing.T) {
	cfg := setupTestPlatformState(t)
	enableOAuth(t, cfg, "roles")

	localToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "alice"})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	var oauthAvailable bool
	original := newOIDCClaimsVerifier
	t.Cleanup(func() { newOIDCClaimsVerifier = original })
	newOIDCClaimsVerifier = func(ctx context.Context, conf *Config) (oidcClaimsVerifier, error) {
		if !oauthAvailable {
			return fakeClaimsVerifier{err: errors.New("idp down")}, nil
		}
		return fakeClaimsVerifier{claims: map[string]interface{}{"sub": "bob"}}, nil
	}

	router, err := newRouter(Routes{{
		Path:   "/either",
		Method: http.MethodGet,
		Auth:   AuthLocalOrOIDC,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/either", nil)
	req.Header.Set("Authorization", "Bearer "+localToken)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected a local JWT to be accepted, got %d", res.Code)
	}

	oauthAvailable = true
	req = httptest.NewRequest(http.MethodGet, "/either", nil)
	req.Header.Set("Authorization", "Bearer opaque-idp-token")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected an OIDC token to be accepted, got %d", res.Code)
	}
}

func TestRouteWithoutAuthStrategyFailsClosed(t *testing.T) {
	setupTestPlatformState(t)

	_, err := newRouter(Routes{{
		Path:        "/public-but-unspecified",
		Method:      http.MethodGet,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {},
	}})
	if !errors.Is(err, ErrRouteAuthStrategyRequired) {
		t.Fatalf("expected ErrRouteAuthStrategyRequired, got %v", err)
	}
}

func TestAuthNoneRouteIsExplicitlyPublic(t *testing.T) {
	setupTestPlatformState(t)

	router, err := newRouter(Routes{{
		Path:   "/public",
		Method: http.MethodGet,
		Auth:   AuthNone,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/public", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected an unauthenticated call to succeed, got %d", response.Code)
	}
}

func TestLocalOrOIDCRequiresBothStrategiesEnabled(t *testing.T) {
	cfg := setupTestPlatformState(t)
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.OAuth.Enabled = false
	SetPlatformConfiguration(*cfg)

	_, err := newRouter(Routes{{
		Path:        "/either",
		Method:      http.MethodGet,
		Auth:        AuthLocalOrOIDC,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {},
	}})
	if err == nil {
		t.Fatal("expected local-or-oidc to fail when oauth is disabled")
	}
}

func TestRouteWithoutAuthStrategyFailsClosedAtRouterBuild(t *testing.T) {
	// Guard against a future change that makes an empty strategy mean public.
	if _, err := routeAuthStrategies(Route{}); !errors.Is(err, ErrRouteAuthStrategyRequired) {
		t.Fatalf("expected ErrRouteAuthStrategyRequired, got %v", err)
	}
}

func TestRouterRejectsUnknownOrDisabledStrategies(t *testing.T) {
	cfg := setupTestPlatformState(t)
	handler := func(w http.ResponseWriter, r *http.Request) {}

	if _, err := newRouter(Routes{{Path: "/x", Method: http.MethodGet, Auth: AuthStrategy("nonsense"), HandlerFunc: handler}}); err == nil {
		t.Fatal("expected an error for an unknown auth strategy")
	}

	cfg.Auth.Server.OAuth.Enabled = false
	SetPlatformConfiguration(*cfg)
	if _, err := newRouter(Routes{{Path: "/x", Method: http.MethodGet, Auth: AuthOAuth, HandlerFunc: handler}}); err == nil {
		t.Fatal("expected an error when oauth is requested but disabled")
	}
}

func TestBasicAuthStrategy(t *testing.T) {
	cfg := setupTestPlatformState(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	cfg.Auth.Server.Basic.Enabled = true
	cfg.Auth.Server.Basic.AllowedUsers = map[string]string{"alice": string(hash)}
	SetPlatformConfiguration(*cfg)

	router, err := newRouter(Routes{{
		Path:   "/basic",
		Method: http.MethodGet,
		Auth:   AuthBasic,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/basic", nil)
	req.SetBasicAuth("alice", "s3cret")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with correct basic credentials, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/basic", nil)
	req.SetBasicAuth("alice", "wrong")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong basic credentials, got %d", res.Code)
	}
}

func TestRecoveryMiddlewareReturnsJSON500WithRequestID(t *testing.T) {
	setupTestPlatformState(t)

	router, err := newRouter(Routes{{
		Path:   "/boom",
		Method: http.MethodGet,
		Auth:   AuthNone,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			panic("exploded")
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", res.Code)
	}

	requestID := res.Header().Get(RequestIDHeader)
	if requestID == "" {
		t.Fatal("expected an X-Request-ID response header")
	}

	var body map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected a JSON error body, got %q: %v", res.Body.String(), err)
	}
	if body["request_id"] != requestID {
		t.Fatalf("expected body request_id %q, got %q", requestID, body["request_id"])
	}
}

func TestRequestIDMiddlewarePreservesIncomingValue(t *testing.T) {
	setupTestPlatformState(t)

	var seen string
	router, err := newRouter(Routes{{
		Path:   "/id",
		Method: http.MethodGet,
		Auth:   AuthNone,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			seen = RequestIDFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/id", nil)
	req.Header.Set(RequestIDHeader, "caller-supplied-id")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if seen != "caller-supplied-id" {
		t.Fatalf("expected the incoming request id to be preserved, got %q", seen)
	}
	if res.Header().Get(RequestIDHeader) != "caller-supplied-id" {
		t.Fatalf("expected the request id on the response, got %q", res.Header().Get(RequestIDHeader))
	}
}
