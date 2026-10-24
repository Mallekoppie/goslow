package platform

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	jwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	identityEchoProcedure       = "/test.IdentityEchoService/Echo"
	identityEchoStreamProcedure = "/test.IdentityEchoService/Stream"
)

// identityEchoService reports the identity the authorizer stored for the
// procedure, so tests can assert on what a real handler observes.
type identityEchoService struct{}

func (s *identityEchoService) Register(mux *http.ServeMux, options ...connect.HandlerOption) {
	mux.Handle(identityEchoProcedure, connect.NewUnaryHandler(
		identityEchoProcedure,
		func(ctx context.Context, request *connect.Request[wrapperspb.StringValue]) (*connect.Response[wrapperspb.StringValue], error) {
			return connect.NewResponse(wrapperspb.String(describeIdentity(ctx, request.Msg.GetValue()))), nil
		},
		options...,
	))

	mux.Handle(identityEchoStreamProcedure, connect.NewServerStreamHandler(
		identityEchoStreamProcedure,
		func(ctx context.Context, request *connect.Request[wrapperspb.StringValue], stream *connect.ServerStream[wrapperspb.StringValue]) error {
			return stream.Send(wrapperspb.String(describeIdentity(ctx, request.Msg.GetValue())))
		},
		options...,
	))
}

func describeIdentity(ctx context.Context, prefix string) string {
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		return prefix + "|no-identity"
	}

	description := prefix + "|user=" + identity.Username + "|roles=" + strings.Join(identity.Roles, ",")
	if identity.ClientToken != "" {
		description += "|client-token"
	}
	return description
}

func newIdentityEchoHandler(t *testing.T, options ConnectServerOptions) http.Handler {
	t.Helper()
	options.Services = append(options.Services, &identityEchoService{})
	handler, err := NewConnectHandler(options)
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	return handler
}

func callIdentityEcho(t *testing.T, client *connect.Client[wrapperspb.StringValue, wrapperspb.StringValue], token string) (string, error) {
	t.Helper()

	request := connect.NewRequest(wrapperspb.String("hi"))
	if token != "" {
		request.Header().Set("Authorization", "Bearer "+token)
	}
	response, err := client.CallUnary(context.Background(), request)
	if err != nil {
		return "", err
	}
	return response.Msg.GetValue(), nil
}

func TestConnectAuthStoresTheIdentityInTheProcedureContext(t *testing.T) {
	configureConnectLocalJWT(t)

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"preferred_username": "alice",
		"roles":              []string{"admin"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	server := httptest.NewServer(newIdentityEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+identityEchoProcedure)
	value, err := callIdentityEcho(t, client, token)
	if err != nil {
		t.Fatalf("expected the token to be accepted: %v", err)
	}

	const want = "hi|user=alice|roles=admin|client-token"
	if value != want {
		t.Fatalf("expected %q, got %q", want, value)
	}
}

func TestConnectProcedureRolesUseTheConfiguredRolesClaim(t *testing.T) {
	cfg := configureConnectLocalJWT(t)
	cfg.Auth.Server.LocalJwt.RolesClaim = "permissions"
	cfg.Grpc.Server.ProcedureRoles = map[string][]string{
		identityEchoProcedure: {"admin", "owner"},
	}
	SetPlatformConfiguration(*cfg)

	server := httptest.NewServer(newIdentityEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()
	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+identityEchoProcedure)

	ownerToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"permissions": []string{"owner"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	if _, err := callIdentityEcho(t, client, ownerToken); err != nil {
		t.Fatalf("expected any matching configured role to pass: %v", err)
	}

	// The same role under a different claim is not the configured claim.
	otherClaimToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"roles": []string{"owner"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	_, err = callIdentityEcho(t, client, otherClaimToken)
	if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
		t.Fatalf("expected CodePermissionDenied for a role outside the configured claim, got %v (%v)", got, err)
	}
}

func TestConnectAuthAppliesAcrossProtocols(t *testing.T) {
	configureConnectLocalJWT(t)

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"preferred_username": "alice",
		"roles":              []string{"admin"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	server := newH2CTestServer(t, newIdentityEchoHandler(t, ConnectServerOptions{}))

	protocols := []struct {
		name    string
		client  *http.Client
		options []connect.ClientOption
	}{
		{name: "connect"},
		{name: "grpc-web", options: []connect.ClientOption{connect.WithGRPCWeb()}},
		{name: "grpc-h2c", client: h2cHTTPClient(), options: []connect.ClientOption{connect.WithGRPC()}},
	}

	for _, protocol := range protocols {
		t.Run(protocol.name, func(t *testing.T) {
			httpClient := server.Client()
			if protocol.client != nil {
				httpClient = protocol.client
			}
			client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
				httpClient, server.URL+identityEchoProcedure, protocol.options...)

			value, err := callIdentityEcho(t, client, token)
			if err != nil {
				t.Fatalf("expected the %s token to be accepted: %v", protocol.name, err)
			}
			const want = "hi|user=alice|roles=admin|client-token"
			if value != want {
				t.Fatalf("expected %q, got %q", want, value)
			}

			_, err = callIdentityEcho(t, client, "")
			if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
				t.Fatalf("expected CodeUnauthenticated without a credential, got %v (%v)", got, err)
			}
		})
	}
}

func TestConnectStreamingHandlerAuthorizesThroughTheAuthorizer(t *testing.T) {
	configureConnectLocalJWT(t)

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"preferred_username": "alice",
		"roles":              []string{"admin"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	server := httptest.NewServer(newIdentityEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+identityEchoStreamProcedure)

	request := connect.NewRequest(wrapperspb.String("hi"))
	request.Header().Set("Authorization", "Bearer "+token)
	stream, err := client.CallServerStream(context.Background(), request)
	if err != nil {
		t.Fatalf("CallServerStream failed: %v", err)
	}
	defer stream.Close()

	if !stream.Receive() {
		t.Fatalf("expected a streamed message, got error %v", stream.Err())
	}
	const want = "hi|user=alice|roles=admin|client-token"
	if got := stream.Msg().GetValue(); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if stream.Receive() {
		t.Fatal("expected the stream to end after one message")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}
}

func TestConnectAuthorizationErrorPrecedence(t *testing.T) {
	cfg := setupTestPlatformState(t)
	enableOAuth(t, cfg, "roles")
	cfg.Grpc.Server.Auth = AuthLocalOrOIDC
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}
	stubOIDCVerifier(t, nil, errors.New("idp down"))

	server := httptest.NewServer(newIdentityEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()
	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+identityEchoProcedure)

	// No strategy can judge the credential and the provider is down: that is an
	// outage, not a rejected credential.
	_, err := callIdentityEcho(t, client, "opaque-token")
	if got := connect.CodeOf(err); got != connect.CodeUnavailable {
		t.Fatalf("expected CodeUnavailable when no strategy rejects the credential, got %v (%v)", got, err)
	}

	// A local JWT with a valid shape but a bad signature is a definite
	// rejection, so the provider outage must not mask it.
	definitelyInvalid := signLocalJWT(t, "wrong-signing-key", jwt.SigningMethodHS256, map[string]interface{}{
		"sub": "alice",
	})
	_, err = callIdentityEcho(t, client, definitelyInvalid)
	if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated when a strategy definitely rejects, got %v (%v)", got, err)
	}
}

func TestConnectHandlerRejectsBasicAuthStrategyAtConstruction(t *testing.T) {
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Auth.Server.Basic.Enabled = true
	cfg.Auth.Server.Basic.AllowedUsers = map[string]string{"alice": "unused"}
	cfg.Grpc.Server.Auth = AuthBasic
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	if _, err := NewConnectHandler(ConnectServerOptions{}); err == nil {
		t.Fatal("expected a basic RPC strategy to fail handler construction")
	}
}

func TestConnectHandlerRejectsProcedureRolesWithoutAnAuthStrategy(t *testing.T) {
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Grpc.Server.Auth = AuthNone
	cfg.Grpc.Server.ProcedureRoles = map[string][]string{
		identityEchoProcedure: {"admin"},
	}
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	if _, err := NewConnectHandler(ConnectServerOptions{}); err == nil {
		t.Fatal("expected required roles without an auth strategy to fail handler construction")
	}
}

func TestConnectHandlerRejectsExcludedProcedureWithRequiredRoles(t *testing.T) {
	cfg := configureConnectLocalJWT(t)
	cfg.Auth.Server.UnauthenticatedPaths = []string{identityEchoProcedure}
	cfg.Grpc.Server.ProcedureRoles = map[string][]string{
		identityEchoProcedure: {"admin"},
	}
	SetPlatformConfiguration(*cfg)

	if _, err := NewConnectHandler(ConnectServerOptions{}); err == nil {
		t.Fatal("expected an excluded procedure with required roles to fail handler construction")
	}
}

func TestConnectStreamingHandlerEnforcesProcedureRoles(t *testing.T) {
	cfg := configureConnectLocalJWT(t)
	cfg.Grpc.Server.ProcedureRoles = map[string][]string{
		identityEchoStreamProcedure: {"admin"},
	}
	SetPlatformConfiguration(*cfg)

	server := httptest.NewServer(newIdentityEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"roles": []string{"reader"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+identityEchoStreamProcedure)
	request := connect.NewRequest(wrapperspb.String("hi"))
	request.Header().Set("Authorization", "Bearer "+token)
	stream, err := client.CallServerStream(context.Background(), request)
	if err != nil {
		t.Fatalf("CallServerStream failed: %v", err)
	}
	defer stream.Close()

	if stream.Receive() {
		t.Fatal("expected the streaming handler to reject the request")
	}
	if got := connect.CodeOf(stream.Err()); got != connect.CodePermissionDenied {
		t.Fatalf("expected CodePermissionDenied, got %v (%v)", got, stream.Err())
	}
}

func TestApplicationAuthorizesHTTPAndRPCThroughOneAuthorizer(t *testing.T) {
	cfg := setupTestPlatformState(t)
	cfg.Grpc.Server.Auth = AuthLocalJWT
	cfg.Grpc.Server.ProcedureRoles = map[string][]string{
		identityEchoProcedure: {"admin"},
	}
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	var httpIdentity Identity
	var httpHasIdentity bool
	handler, err := NewApplicationHandler(ApplicationServerOptions{
		Services: []ConnectService{&identityEchoService{}},
		Routes: Routes{{
			Path:          "/admin",
			Method:        http.MethodGet,
			Auth:          AuthLocalJWT,
			RolesRequired: []string{"admin"},
			HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
				httpIdentity, httpHasIdentity = IdentityFromContext(r.Context())
				w.WriteHeader(http.StatusNoContent)
			},
		}},
	})
	if err != nil {
		t.Fatalf("NewApplicationHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	rpcClient := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+identityEchoProcedure)

	adminToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"preferred_username": "alice",
		"roles":              []string{"admin"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	readerToken, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"preferred_username": "bob",
		"roles":              []string{"reader"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	callHTTP := func(token string) int {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, server.URL+"/admin", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}

	// The same admin credential passes both adapters.
	value, err := callIdentityEcho(t, rpcClient, adminToken)
	if err != nil {
		t.Fatalf("expected the RPC procedure to accept the admin token: %v", err)
	}
	if value != "hi|user=alice|roles=admin|client-token" {
		t.Fatalf("unexpected RPC identity %q", value)
	}
	if status := callHTTP(adminToken); status != http.StatusNoContent {
		t.Fatalf("expected the HTTP route to accept the admin token, got %d", status)
	}
	if !httpHasIdentity || httpIdentity.Username != "alice" {
		t.Fatalf("expected the HTTP handler to read the shared identity, got %+v", httpIdentity)
	}

	// A verified caller without the role is forbidden on both transports.
	if _, err := callIdentityEcho(t, rpcClient, readerToken); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("expected CodePermissionDenied for the RPC procedure, got %v", connect.CodeOf(err))
	}
	if status := callHTTP(readerToken); status != http.StatusForbidden {
		t.Fatalf("expected 403 for the HTTP route, got %d", status)
	}

	// A missing credential requires authentication on both transports.
	if _, err := callIdentityEcho(t, rpcClient, ""); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated for the RPC procedure, got %v", connect.CodeOf(err))
	}
	if status := callHTTP(""); status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for the HTTP route, got %d", status)
	}
}

func TestApplicationSharesOneProviderCacheAcrossAdapters(t *testing.T) {
	cfg := setupTestPlatformState(t)
	clearOIDCProviderCache()
	idp := newFakeIDP(t)

	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = idp.server.URL
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	cfg.Auth.Server.OAuth.AllowedAlgorithms = []string{"RS256"}
	cfg.Grpc.Server.Auth = AuthOAuth
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	handler, err := NewApplicationHandler(ApplicationServerOptions{
		Services: []ConnectService{&identityEchoService{}},
		Routes: Routes{{
			Path:   "/secure",
			Method: http.MethodGet,
			Auth:   AuthOAuth,
			HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
		}},
	})
	if err != nil {
		t.Fatalf("NewApplicationHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	token := idp.signToken(t, idp.validClaims())

	if _, err := callIdentityEcho(t, connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+identityEchoProcedure), token); err != nil {
		t.Fatalf("expected the RPC procedure to accept the ID token: %v", err)
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/secure", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("expected the HTTP route to accept the ID token, got %d", response.StatusCode)
	}

	if hits := atomic.LoadInt32(&idp.discoveryHits); hits != 1 {
		t.Fatalf("expected one provider discovery shared by both adapters, got %d", hits)
	}
}

func TestConnectAuthFailureIsLoggedOnceWithoutTokenValues(t *testing.T) {
	cfg := configureConnectLocalJWT(t)
	cfg.Grpc.Server.ProcedureRoles = map[string][]string{
		identityEchoProcedure: {"admin"},
	}
	SetPlatformConfiguration(*cfg)

	var buffer bytes.Buffer
	previous := Log.current.Load()
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&buffer), zap.InfoLevel)
	Log.current.Store(zap.New(core))
	t.Cleanup(func() { Log.current.Store(previous) })

	server := httptest.NewServer(newIdentityEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"roles": []string{"reader"},
	})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+identityEchoProcedure)
	_, err = callIdentityEcho(t, client, token)
	if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
		t.Fatalf("expected CodePermissionDenied, got %v (%v)", got, err)
	}

	logged := buffer.String()
	if count := strings.Count(logged, `"msg":"RPC completed"`); count != 1 {
		t.Fatalf("expected exactly one access log line, got %d:\n%s", count, logged)
	}
	if strings.Contains(logged, token) {
		t.Fatal("access logging must not record the bearer token")
	}
}
