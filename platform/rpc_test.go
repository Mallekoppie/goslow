package platform

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func newSPATestServer(t *testing.T) *httptest.Server {
	t.Helper()

	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<html>spa-index</html>")},
		"assets/app.js": {Data: []byte("console.log('app')")},
	}

	withTestDB(t)
	handler, err := NewConnectHandler(ConnectServerOptions{
		Services:              []ConnectService{&echoService{}},
		DisableAuthFromConfig: true,
		SPA:                   &SPAOptions{Assets: assets, Index: "index.html"},
	})
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}

	return httptest.NewServer(handler)
}

func TestUnknownRPCNeverFallsThroughToAssets(t *testing.T) {
	withTestDB(t)

	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "asset-body")
	})
	handler, err := NewConnectHandler(ConnectServerOptions{
		Services:              []ConnectService{&echoService{}},
		DisableAuthFromConfig: true,
		WebHandler:            assets,
	})
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	// RPC-shaped path with an RPC content type.
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/wdid.v1.Missing/Method", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/grpc-web+proto")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown RPC, got %d (%q)", response.StatusCode, body)
	}
	if strings.Contains(string(body), "asset-body") {
		t.Fatal("unknown RPC fell through to the web handler")
	}

	// Connect JSON with the protocol header is also RPC-shaped.
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/wdid.v1.Missing/Method", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown Connect JSON RPC, got %d", response.StatusCode)
	}

	// Genuine non-RPC requests still reach the web handler.
	response, err = server.Client().Get(server.URL + "/index.html")
	if err != nil {
		t.Fatalf("asset request failed: %v", err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "asset-body" {
		t.Fatalf("expected the web handler body, got %q", body)
	}
}

func TestSPAFallbackServesIndexAndAssets(t *testing.T) {
	server := newSPATestServer(t)
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/today")
	if err != nil {
		t.Fatalf("SPA route failed: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), "spa-index") {
		t.Fatalf("expected the index fallback, got %q", body)
	}

	response, err = server.Client().Get(server.URL + "/assets/app.js")
	if err != nil {
		t.Fatalf("asset request failed: %v", err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), "console.log") {
		t.Fatalf("expected the asset body, got %q", body)
	}

	// RPC still routes to the RPC handler.
	client := echoClient(server, server.Client())
	if _, err := client.CallUnary(t.Context(), connect.NewRequest(wrapperspb.String("spa"))); err != nil {
		t.Fatalf("RPC failed alongside SPA fallback: %v", err)
	}
}

func TestUnknownRPCDoesNotReturnIndexHTML(t *testing.T) {
	server := newSPATestServer(t)
	defer server.Close()

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/wdid.v1.Missing/Method", strings.NewReader("{}"))
	request.Header.Set("Connect-Protocol-Version", "1")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.StatusCode)
	}
	if strings.Contains(string(body), "spa-index") {
		t.Fatal("unknown RPC returned the SPA index")
	}
}

func TestHealthEndpoint(t *testing.T) {
	withTestDB(t)
	cfg, _ := GetPlatformConfiguration()
	cfg.Grpc.Server.HealthEnabled = true
	SetPlatformConfiguration(*cfg)

	handler, err := NewConnectHandler(ConnectServerOptions{
		Services:              []ConnectService{&echoService{}},
		ServiceNames:          []string{"test.EchoService"},
		DisableAuthFromConfig: true,
	})
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/grpc.health.v1.Health/Check", strings.NewReader(`{"service":""}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from health, got %d (%q)", response.StatusCode, body)
	}
	if !strings.Contains(string(body), "SERVING") {
		t.Fatalf("expected SERVING status, got %q", body)
	}

	// Disabled health must not be reachable.
	cfg.Grpc.Server.HealthEnabled = false
	SetPlatformConfiguration(*cfg)
	handler, err = NewConnectHandler(ConnectServerOptions{DisableAuthFromConfig: true})
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	disabled := httptest.NewServer(handler)
	defer disabled.Close()

	response, err = disabled.Client().Post(disabled.URL+"/grpc.health.v1.Health/Check", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("disabled health request failed: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when health is disabled, got %d", response.StatusCode)
	}
}

func TestReflectionEndpointToggle(t *testing.T) {
	withTestDB(t)
	cfg, _ := GetPlatformConfiguration()
	cfg.Grpc.Server.ReflectionEnabled = true
	SetPlatformConfiguration(*cfg)

	handler, err := NewConnectHandler(ConnectServerOptions{
		Services:              []ConnectService{&echoService{}},
		ServiceNames:          []string{"test.EchoService"},
		DisableAuthFromConfig: true,
	})
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", strings.NewReader(""))
	request.Header.Set("Content-Type", "application/grpc-web+proto")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("reflection request failed: %v", err)
	}
	response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		t.Fatal("expected reflection to be registered when enabled")
	}

	cfg.Grpc.Server.ReflectionEnabled = false
	SetPlatformConfiguration(*cfg)
	handler, err = NewConnectHandler(ConnectServerOptions{DisableAuthFromConfig: true})
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	disabled := httptest.NewServer(handler)
	defer disabled.Close()

	request, _ = http.NewRequest(http.MethodPost, disabled.URL+"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", strings.NewReader(""))
	request.Header.Set("Content-Type", "application/grpc-web+proto")
	response, err = disabled.Client().Do(request)
	if err != nil {
		t.Fatalf("disabled reflection request failed: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when reflection is disabled, got %d", response.StatusCode)
	}
}
