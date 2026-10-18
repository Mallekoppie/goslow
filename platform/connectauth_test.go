package platform

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func configureConnectLocalJWT(t *testing.T) *Config {
	t.Helper()
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "connect-test-key"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	cfg.Grpc.Server.Auth = AuthLocalJWT
	SetPlatformConfiguration(*cfg)
	return cfg
}

func callEcho(t *testing.T, client *connect.Client[wrapperspb.StringValue, wrapperspb.StringValue], token string) (string, error) {
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

func TestConnectAuthRequiresBearerToken(t *testing.T) {
	configureConnectLocalJWT(t)

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := echoClient(server, server.Client())
	outcomes := []struct {
		name  string
		token string
	}{
		{name: "missing"},
		{name: "malformed", token: "not-a-jwt"},
	}

	for _, outcome := range outcomes {
		t.Run(outcome.name, func(t *testing.T) {
			_, err := callEcho(t, client, outcome.token)
			if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
				t.Fatalf("expected CodeUnauthenticated, got %v (%v)", got, err)
			}
		})
	}
}

func TestConnectAuthAcceptsLocalJWTAndPropagatesClaims(t *testing.T) {
	configureConnectLocalJWT(t)

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "alice"})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := echoClient(server, server.Client())
	value, err := callEcho(t, client, token)
	if err != nil {
		t.Fatalf("expected the token to be accepted: %v", err)
	}
	if value != "hi|alice" {
		t.Fatalf("expected claims in the handler context, got %q", value)
	}
}

func TestConnectAuthUnauthenticatedPathBypassesTokenCheck(t *testing.T) {
	cfg := configureConnectLocalJWT(t)
	cfg.Auth.Server.UnauthenticatedPaths = []string{echoProcedure}
	SetPlatformConfiguration(*cfg)

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := echoClient(server, server.Client())
	if _, err := callEcho(t, client, ""); err != nil {
		t.Fatalf("expected the public procedure to be callable without a token: %v", err)
	}
}

func TestConnectAuthUsesOAuthWhenLocalJWTDisabled(t *testing.T) {
	resetPlatformState(t)
	idp := newFakeIDP(t)

	cfg := createDefaultConfiguration()
	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = idp.server.URL
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	cfg.Auth.Server.OAuth.AllowedAlgorithms = []string{"RS256"}
	cfg.Grpc.Server.Auth = AuthOAuth
	SetPlatformConfiguration(*cfg)

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := echoClient(server, server.Client())
	value, err := callEcho(t, client, idp.signToken(t, idp.validClaims()))
	if err != nil {
		t.Fatalf("expected the OAuth token to be accepted: %v", err)
	}
	if value != "hi|alice" {
		t.Fatalf("expected the preferred username in context, got %q", value)
	}
}

func TestConnectAuthReportsIdentityProviderOutage(t *testing.T) {
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = "http://127.0.0.1:1/.well-known/openid-configuration"
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	cfg.Grpc.Server.Auth = AuthOAuth
	SetPlatformConfiguration(*cfg)

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := echoClient(server, server.Client())
	_, err := callEcho(t, client, "opaque-token")
	if got := connect.CodeOf(err); got != connect.CodeUnavailable {
		t.Fatalf("expected CodeUnavailable, got %v (%v)", got, err)
	}
}

func TestConnectStreamAuthRejectsMissingToken(t *testing.T) {
	configureConnectLocalJWT(t)

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+echoStreamProcedure)

	stream, err := client.CallServerStream(context.Background(), connect.NewRequest(wrapperspb.String("x")))
	if err != nil {
		t.Fatalf("CallServerStream failed: %v", err)
	}
	defer stream.Close()

	if stream.Receive() {
		t.Fatal("expected the streaming handler to reject the request")
	}
	if got := connect.CodeOf(stream.Err()); got != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v (%v)", got, stream.Err())
	}
}

func TestConnectAuthDisabledAllowsAnonymousCalls(t *testing.T) {
	configureConnectLocalJWT(t)

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{DisableAuthFromConfig: true}))
	defer server.Close()

	client := echoClient(server, server.Client())
	value, err := callEcho(t, client, "")
	if err != nil {
		t.Fatalf("expected anonymous access when auth is disabled: %v", err)
	}
	if value != "hi" {
		t.Fatalf("unexpected response %q", value)
	}
}

func TestConnectAuthStrategyErrorMessage(t *testing.T) {
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Auth.Server.Basic.Enabled = true
	SetPlatformConfiguration(*cfg)

	_, err := newConnectAuthInterceptor(cfg)
	if err == nil {
		t.Fatal("expected an error for basic-only configuration")
	}
	if got := fmt.Sprintf("%v", err); got == "" {
		t.Fatal("expected a descriptive error message")
	}
}
