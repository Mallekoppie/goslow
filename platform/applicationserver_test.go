package platform

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestApplicationHandlerServesRPCAndHTTPAndAssets(t *testing.T) {
	withTestDB(t)

	assets := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}

	handler, err := NewApplicationHandler(ApplicationServerOptions{
		Services:              []ConnectService{&echoService{}},
		DisableAuthFromConfig: true,
		Routes: Routes{{
			Path:   "/api/health",
			Method: http.MethodGet,
			Auth:   AuthNone,
			HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("http-route"))
			},
		}},
		SPA: &SPAOptions{Assets: assets, Index: "index.html"},
	})
	if err != nil {
		t.Fatalf("NewApplicationHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	// RPC
	client := echoClient(server, server.Client())
	response, err := client.CallUnary(context.Background(), connect.NewRequest(wrapperspb.String("app")))
	if err != nil {
		t.Fatalf("RPC failed: %v", err)
	}
	if response.Msg.GetValue() != "app" {
		t.Fatalf("unexpected RPC response %q", response.Msg.GetValue())
	}

	// HTTP route
	httpResponse, err := server.Client().Get(server.URL + "/api/health")
	if err != nil {
		t.Fatalf("HTTP route failed: %v", err)
	}
	body, _ := io.ReadAll(httpResponse.Body)
	httpResponse.Body.Close()
	if string(body) != "http-route" {
		t.Fatalf("unexpected HTTP route body %q", body)
	}

	// SPA fallback
	httpResponse, err = server.Client().Get(server.URL + "/today")
	if err != nil {
		t.Fatalf("SPA route failed: %v", err)
	}
	body, _ = io.ReadAll(httpResponse.Body)
	httpResponse.Body.Close()
	if !strings.Contains(string(body), "app") {
		t.Fatalf("expected the SPA index, got %q", body)
	}

	// Unknown RPC never falls through.
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/wdid.v1.Missing/Method", strings.NewReader("{}"))
	request.Header.Set("Connect-Protocol-Version", "1")
	httpResponse, err = server.Client().Do(request)
	if err != nil {
		t.Fatalf("unknown RPC request failed: %v", err)
	}
	httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown RPC, got %d", httpResponse.StatusCode)
	}
}

func TestReadJsonRequestUsesConfiguredLimit(t *testing.T) {
	cfg := setupTestPlatformState(t)
	cfg.HTTP.Server.MaxRequestBodyBytes = 8
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	var payload map[string]interface{}
	err := JsonMarshaller.ReadJsonRequest(readCloserFrom(`{"value":1}`), &payload)
	if err != ErrRequestBodyTooLarge {
		t.Fatalf("expected ErrRequestBodyTooLarge for configured limit, got %v", err)
	}

	// The per-call override still wins.
	if err := JsonMarshaller.ReadJsonRequestLimit(readCloserFrom(`{"value":1}`), &payload, 1024); err != nil {
		t.Fatalf("expected the explicit limit to apply: %v", err)
	}
}
