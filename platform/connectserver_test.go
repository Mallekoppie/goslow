package platform

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	echoProcedure        = "/test.EchoService/Echo"
	echoStreamProcedure  = "/test.EchoService/Stream"
	connectTestAllowList = "https://app.example"
)

type echoService struct{}

func (e *echoService) Register(mux *http.ServeMux, options ...connect.HandlerOption) {
	mux.Handle(echoProcedure, connect.NewUnaryHandler(
		echoProcedure,
		func(ctx context.Context, request *connect.Request[wrapperspb.StringValue]) (*connect.Response[wrapperspb.StringValue], error) {
			value := request.Msg.GetValue()
			if identity, ok := IdentityFromContext(ctx); ok {
				if identity.Username != "" {
					value += "|" + identity.Username
				}
				if subject, ok := identity.Claims["sub"].(string); ok {
					value += "|" + subject
				}
			}
			return connect.NewResponse(wrapperspb.String(value)), nil
		},
		options...,
	))

	mux.Handle(echoStreamProcedure, connect.NewServerStreamHandler(
		echoStreamProcedure,
		func(ctx context.Context, request *connect.Request[wrapperspb.StringValue], stream *connect.ServerStream[wrapperspb.StringValue]) error {
			return stream.Send(wrapperspb.String(request.Msg.GetValue()))
		},
		options...,
	))
}

func newEchoHandler(t *testing.T, options ConnectServerOptions) http.Handler {
	t.Helper()
	options.Services = append(options.Services, &echoService{})
	handler, err := NewConnectHandler(options)
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	return handler
}

func h2cHTTPClient() *http.Client {
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Client{Transport: &http.Transport{Protocols: protocols}}
}

// newH2CTestServer serves the handler over HTTP/1.1 and unencrypted HTTP/2 so
// the Connect, gRPC-Web, and native gRPC clients can share one listener.
func newH2CTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	configureHTTPProtocols(server.Config, false)
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func echoClient(server *httptest.Server, client *http.Client, options ...connect.ClientOption) *connect.Client[wrapperspb.StringValue, wrapperspb.StringValue] {
	baseURL := server.URL
	if client == nil {
		client = server.Client()
	}
	return connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](client, baseURL+echoProcedure, options...)
}

func TestConnectHandlerServesAllProtocols(t *testing.T) {
	withTestDB(t) // resets global state without touching gRPC configuration
	handler := newEchoHandler(t, ConnectServerOptions{DisableAuthFromConfig: true})
	server := newH2CTestServer(t, handler)

	protocols := []struct {
		name    string
		client  *http.Client
		options []connect.ClientOption
	}{
		{name: "connect", options: nil},
		{name: "grpc-web", options: []connect.ClientOption{connect.WithGRPCWeb()}},
		{name: "grpc-h2c", client: h2cHTTPClient(), options: []connect.ClientOption{connect.WithGRPC()}},
	}

	for _, protocol := range protocols {
		t.Run(protocol.name, func(t *testing.T) {
			client := echoClient(server, protocol.client, protocol.options...)
			response, err := client.CallUnary(context.Background(), connect.NewRequest(wrapperspb.String("hello")))
			if err != nil {
				t.Fatalf("%s call failed: %v", protocol.name, err)
			}
			if got := response.Msg.GetValue(); got != "hello" {
				t.Fatalf("expected hello, got %q", got)
			}
		})
	}
}

func TestConnectHandlerServesStreaming(t *testing.T) {
	withTestDB(t)
	handler := newEchoHandler(t, ConnectServerOptions{DisableAuthFromConfig: true})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+echoStreamProcedure)
	stream, err := client.CallServerStream(context.Background(), connect.NewRequest(wrapperspb.String("stream")))
	if err != nil {
		t.Fatalf("CallServerStream failed: %v", err)
	}
	defer stream.Close()

	if !stream.Receive() {
		t.Fatalf("expected a streamed message, got error %v", stream.Err())
	}
	if got := stream.Msg().GetValue(); got != "stream" {
		t.Fatalf("expected stream, got %q", got)
	}
	if stream.Receive() {
		t.Fatal("expected the stream to end after one message")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}
}

func TestConnectHandlerCORSPreflight(t *testing.T) {
	withTestDB(t)
	handler := newEchoHandler(t, ConnectServerOptions{
		DisableAuthFromConfig: true,
		AllowedOrigins:        []string{connectTestAllowList},
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	request, err := http.NewRequest(http.MethodOptions, server.URL+echoProcedure, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Origin", connectTestAllowList)
	request.Header.Set("Access-Control-Request-Method", "POST")
	request.Header.Set("Access-Control-Request-Headers", "content-type,x-grpc-web")

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.StatusCode)
	}
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != connectTestAllowList {
		t.Fatalf("expected the allow-listed origin, got %q", got)
	}
	allowedHeaders := strings.ToLower(response.Header.Get("Access-Control-Allow-Headers"))
	for _, required := range []string{"content-type", "x-grpc-web", "authorization"} {
		if !strings.Contains(allowedHeaders, required) {
			t.Fatalf("expected %q in allowed headers %q", required, allowedHeaders)
		}
	}

	// A disallowed origin gets no CORS grant.
	request, _ = http.NewRequest(http.MethodOptions, server.URL+echoProcedure, nil)
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Access-Control-Request-Method", "POST")
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	response.Body.Close()
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS grant for a foreign origin, got %q", got)
	}
}

func TestConnectHandlerServesWebAssetsAndFallback(t *testing.T) {
	withTestDB(t)
	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "flutter-asset:"+r.URL.Path)
	})

	handler := newEchoHandler(t, ConnectServerOptions{
		DisableAuthFromConfig: true,
		WebHandler:            assets,
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/index.html")
	if err != nil {
		t.Fatalf("asset request failed: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "flutter-asset:/index.html" {
		t.Fatalf("unexpected asset body %q", body)
	}

	// RPCs still route to the connect handler.
	client := echoClient(server, server.Client())
	if _, err := client.CallUnary(context.Background(), connect.NewRequest(wrapperspb.String("x"))); err != nil {
		t.Fatalf("rpc with web fallback failed: %v", err)
	}
}

func TestConnectHandlerUnknownPathWithoutWebHandler(t *testing.T) {
	withTestDB(t)
	handler := newEchoHandler(t, ConnectServerOptions{DisableAuthFromConfig: true})
	server := httptest.NewServer(handler)
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/not-an-rpc")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.StatusCode)
	}
}

func TestConnectHandlerRejectsBasicOnlyAuth(t *testing.T) {
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Auth.Server.Basic.Enabled = true
	SetPlatformConfiguration(*cfg)

	if _, err := NewConnectHandler(ConnectServerOptions{}); err == nil {
		t.Fatal("expected basic-only auth to be rejected for gRPC transports")
	}
}

func TestServeConnectShutsDownWithContext(t *testing.T) {
	withTestDB(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- ServeConnect(ctx, listener, ConnectServerOptions{
			Services:              []ConnectService{&echoService{}},
			DisableAuthFromConfig: true,
		})
	}()

	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		&http.Client{Timeout: time.Second}, "http://"+listener.Addr().String()+echoProcedure)

	deadline := time.Now().Add(5 * time.Second)
	var callErr error
	for time.Now().Before(deadline) {
		ctxCall, stop := context.WithTimeout(context.Background(), time.Second)
		_, callErr = client.CallUnary(ctxCall, connect.NewRequest(wrapperspb.String("ping")))
		stop()
		if callErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if callErr != nil {
		cancel()
		t.Fatalf("server never became reachable: %v", callErr)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("expected a clean shutdown, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ServeConnect did not return after cancellation")
	}
}
