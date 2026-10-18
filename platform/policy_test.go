package platform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
)

func TestConnectRolePolicy(t *testing.T) {
	cfg := configureConnectLocalJWT(t)
	cfg.Grpc.Server.ProcedureRoles = map[string][]string{echoProcedure: {"admin"}}
	SetPlatformConfiguration(*cfg)

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()
	client := echoClient(server, server.Client())

	adminToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"sub":   "alice",
		"roles": []string{"admin"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	if _, err := callEcho(t, client, adminToken); err != nil {
		t.Fatalf("expected the admin role to be accepted: %v", err)
	}

	userToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"sub":   "bob",
		"roles": []string{"user"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	_, err = callEcho(t, client, userToken)
	if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
		t.Fatalf("expected CodePermissionDenied, got %v (%v)", got, err)
	}
}

func TestConnectAuthStrategyValidation(t *testing.T) {
	t.Run("unset strategy with auth enabled fails closed", func(t *testing.T) {
		resetPlatformState(t)
		cfg := createDefaultConfiguration()
		cfg.Auth.Server.LocalJwt.Enabled = true
		SetPlatformConfiguration(*cfg)

		if _, err := newConnectAuthInterceptor(cfg); !errors.Is(err, ErrConnectAuthStrategyRequired) {
			t.Fatalf("expected ErrConnectAuthStrategyRequired, got %v", err)
		}
	})

	t.Run("disabled strategy fails", func(t *testing.T) {
		resetPlatformState(t)
		cfg := createDefaultConfiguration()
		cfg.Grpc.Server.Auth = AuthOAuth
		SetPlatformConfiguration(*cfg)

		if _, err := newConnectAuthInterceptor(cfg); err == nil {
			t.Fatal("expected an error for a disabled oauth strategy")
		}
	})

	t.Run("basic strategy is rejected", func(t *testing.T) {
		resetPlatformState(t)
		cfg := createDefaultConfiguration()
		cfg.Grpc.Server.Auth = AuthBasic
		SetPlatformConfiguration(*cfg)

		if _, err := newConnectAuthInterceptor(cfg); err == nil {
			t.Fatal("expected basic auth to be rejected")
		}
	})

	t.Run("explicit none disables auth", func(t *testing.T) {
		resetPlatformState(t)
		cfg := createDefaultConfiguration()
		cfg.Grpc.Server.Auth = AuthNone
		cfg.Auth.Server.LocalJwt.Enabled = true
		SetPlatformConfiguration(*cfg)

		interceptor, err := newConnectAuthInterceptor(cfg)
		if err != nil || interceptor != nil {
			t.Fatalf("expected no interceptor for AuthNone, got %v / %v", interceptor, err)
		}
	})
}

func TestHTTPExclusionList(t *testing.T) {
	cfg := setupTestPlatformState(t)
	cfg.Auth.Server.UnauthenticatedPaths = []string{"POST /auth/login", "/metrics"}
	SetPlatformConfiguration(*cfg)

	router, err := newRouter(Routes{
		{
			Path:   "/auth/login",
			Method: http.MethodPost,
			// Auth intentionally omitted: the exclusion list declares it public.
			HandlerFunc: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) },
		},
		{
			Path:        "/private",
			Method:      http.MethodGet,
			Auth:        AuthNone,
			HandlerFunc: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) },
		},
	})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("{}")))
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected the excluded route to be public, got %d", response.Code)
	}
}

func TestHTTPExclusionListConflictsWithExplicitAuth(t *testing.T) {
	cfg := setupTestPlatformState(t)
	cfg.Auth.Server.UnauthenticatedPaths = []string{"GET /private"}
	SetPlatformConfiguration(*cfg)

	_, err := newRouter(Routes{{
		Path:        "/private",
		Method:      http.MethodGet,
		Auth:        AuthLocalJWT,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {},
	}})
	if err == nil {
		t.Fatal("expected a conflict error between explicit auth and the exclusion list")
	}
}

func TestHTTPCORSAllowList(t *testing.T) {
	cfg := setupTestPlatformState(t)
	cfg.HTTP.Server.AllowedOrigins = []string{"https://app.example"}
	SetPlatformConfiguration(*cfg)

	router, err := newRouter(Routes{{
		Path:        "/data",
		Method:      http.MethodGet,
		Auth:        AuthNone,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) },
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodOptions, "/data", nil)
	request.Header.Set("Origin", "https://app.example")
	request.Header.Set("Access-Control-Request-Method", "GET")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204 preflight, got %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("expected exact origin, got %q", got)
	}

	request = httptest.NewRequest(http.MethodOptions, "/data", nil)
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Access-Control-Request-Method", "GET")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no grant for a foreign origin, got %q", got)
	}
}

func TestWildcardOriginsAreRejected(t *testing.T) {
	cfg := createDefaultConfiguration()
	cfg.HTTP.Server.AllowedOrigins = []string{"*"}

	if err := SetPlatformConfiguration(*cfg); err == nil {
		t.Fatal("expected wildcard origins to be rejected")
	}
}

func TestConnectHTTPServerHasNoGlobalWriteTimeout(t *testing.T) {
	conf := createDefaultConfiguration()
	conf.Grpc.Server.WriteTimeoutSeconds = 5
	server := newConnectHTTPServer(conf, http.NotFoundHandler())
	if server.WriteTimeout != 0 {
		t.Fatalf("expected WriteTimeout 0 for streaming, got %s", server.WriteTimeout)
	}
}

func TestValidateServingConfiguration(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(*Config)
	}{
		{name: "missing tls cert", configure: func(c *Config) {
			c.HTTP.Server.TLSEnabled = true
		}},
		{name: "invalid http address", configure: func(c *Config) {
			c.HTTP.Server.ListeningAddress = "not-an-address"
		}},
		{name: "invalid grpc address", configure: func(c *Config) {
			c.Grpc.Server.ListeningAddress = "not-an-address"
		}},
		{name: "oauth without well-known url", configure: func(c *Config) {
			c.Auth.Server.OAuth.Enabled = true
			c.Auth.Server.OAuth.ClientID = "id"
			c.Auth.Server.OAuth.AllowedAlgorithms = []string{"RS256"}
		}},
		{name: "oauth without algorithms", configure: func(c *Config) {
			c.Auth.Server.OAuth.Enabled = true
			c.Auth.Server.OAuth.IdpWellKnownURL = "https://idp.example/.well-known/openid-configuration"
			c.Auth.Server.OAuth.ClientID = "id"
		}},
		{name: "local jwt without key", configure: func(c *Config) {
			c.Auth.Server.LocalJwt.Enabled = true
			c.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
		}},
		{name: "local jwt with bad method", configure: func(c *Config) {
			c.Auth.Server.LocalJwt.Enabled = true
			c.Auth.Server.LocalJwt.JwtSigningKey = "key"
			c.Auth.Server.LocalJwt.JwtSigningMethod = "RS256"
		}},
		{name: "metrics without address", configure: func(c *Config) {
			c.Metrics.Enabled = true
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := createDefaultConfiguration()
			testCase.configure(cfg)
			if err := ValidateServingConfiguration(cfg); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestOAuthInvalidEndpointReturnsError(t *testing.T) {
	resetPlatformState(t)
	cfg := createDefaultConfiguration()
	cfg.Auth.Client.OAuth = []clientTokenConfig{{
		ID:               "bad",
		IdpTokenEndpoint: "://missing-scheme",
		ClientID:         "client",
		ClientSecret:     "secret",
	}}
	SetPlatformConfiguration(*cfg)

	if err := initializeOAuthTokenClients(cfg); err != nil {
		t.Fatalf("initializeOAuthTokenClients failed: %v", err)
	}

	_, err := internalGetOAuth2Token("bad")
	if !errors.Is(err, ErrOAuthInvalidEndpoint) {
		t.Fatalf("expected ErrOAuthInvalidEndpoint, got %v", err)
	}
}

func TestConnectUnaryTimeoutAppliesToUnaryOnly(t *testing.T) {
	conf := createDefaultConfiguration()
	conf.Grpc.Server.WriteTimeoutSeconds = 7
	conf.Grpc.Server.StreamTimeoutSeconds = 0

	interceptor := newConnectTimeoutInterceptor(conf)
	timeoutInterceptor, ok := interceptor.(connectTimeoutInterceptor)
	if !ok {
		t.Fatalf("unexpected interceptor type %T", interceptor)
	}
	if timeoutInterceptor.unaryTimeout <= 0 {
		t.Fatal("expected a bounded unary timeout")
	}
	if timeoutInterceptor.streamTimeout != 0 {
		t.Fatal("expected unlimited streams when StreamTimeoutSeconds is zero")
	}
}

func TestConnectStreamTimeoutIsConfigurable(t *testing.T) {
	conf := createDefaultConfiguration()
	conf.Grpc.Server.StreamTimeoutSeconds = 90

	interceptor := newConnectTimeoutInterceptor(conf).(connectTimeoutInterceptor)
	if interceptor.streamTimeout <= 0 {
		t.Fatal("expected the configured stream timeout")
	}
}

func TestRoleExtractionFromLocalClaims(t *testing.T) {
	claims := map[string]interface{}{"roles": []interface{}{"admin", "user"}}
	roles := rolesFromClaims(claims)
	if len(roles) != 2 || roles[0] != "admin" {
		t.Fatalf("unexpected roles: %v", roles)
	}

	roles = rolesFromClaims(map[string]interface{}{"role": "single"})
	if len(roles) != 1 || roles[0] != "single" {
		t.Fatalf("unexpected single role: %v", roles)
	}
}

func TestEchoClientContextUsesAccessors(t *testing.T) {
	// Guards against regressing to string context keys.
	ctx := context.WithValue(context.Background(), contextOAuthRoles, []string{"admin"})
	if got := RolesFromContext(ctx); len(got) != 1 || got[0] != "admin" {
		t.Fatalf("unexpected roles: %v", got)
	}
	if RequestIDFromContext(ctx) != "" {
		t.Fatal("expected no request id")
	}
}
