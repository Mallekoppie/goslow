package platform

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func enableTestMetrics(t *testing.T) *Config {
	t.Helper()
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Metrics.Enabled = true
	cfg.Metrics.ListenAddress = "127.0.0.1:19099"
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}
	return cfg
}

func scrapeMetrics(t *testing.T) string {
	t.Helper()

	recorder := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(recorder.Body)
	return string(body)
}

func TestMetricsCountHTTPRequests(t *testing.T) {
	enableTestMetrics(t)

	router, err := newRouter(Routes{{
		Path:        "/public",
		Method:      http.MethodGet,
		Auth:        AuthNone,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) },
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/public", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("unexpected status %d", response.Code)
	}

	metrics := scrapeMetrics(t)
	if !strings.Contains(metrics, `goslow_http_requests_total{method="GET",route="/public",status="202"}`) {
		t.Fatalf("expected the HTTP request counter, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `goslow_auth_decisions_total{outcome="anonymous",transport="http"}`) {
		t.Fatalf("expected an anonymous auth decision, got:\n%s", metrics)
	}
}

func TestMetricsCountRPCAndAuthDenials(t *testing.T) {
	enableTestMetrics(t)

	cfg, err := GetPlatformConfiguration()
	if err != nil {
		t.Fatalf("GetPlatformConfiguration failed: %v", err)
	}
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "metrics-key"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	cfg.Grpc.Server.Auth = AuthLocalJWT
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := echoClient(server, server.Client())
	_, err = client.CallUnary(context.Background(), connect.NewRequest(wrapperspb.String("x")))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", err)
	}

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "alice"})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	request := connect.NewRequest(wrapperspb.String("x"))
	request.Header().Set("Authorization", "Bearer "+token)
	if _, err := client.CallUnary(context.Background(), request); err != nil {
		t.Fatalf("authenticated call failed: %v", err)
	}

	metrics := scrapeMetrics(t)
	if !strings.Contains(metrics, `goslow_rpc_requests_total`) {
		t.Fatalf("expected RPC counters, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `outcome="denied",transport="connect"`) {
		t.Fatalf("expected a denied auth decision, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `outcome="allowed",transport="connect"`) {
		t.Fatalf("expected an allowed auth decision, got:\n%s", metrics)
	}
}

func TestMetricsAreNotMountedOnTheApplicationOrigin(t *testing.T) {
	enableTestMetrics(t)

	withTestDB(t)
	handler, err := NewConnectHandler(ConnectServerOptions{
		Services:              []ConnectService{&echoService{}},
		DisableAuthFromConfig: true,
	})
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on the application origin, got %d", response.StatusCode)
	}
}

func TestMetricsListenerIsIsolated(t *testing.T) {
	enableTestMetrics(t)

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	address := probe.Addr().String()
	probe.Close()

	cfg, err := GetPlatformConfiguration()
	if err != nil {
		t.Fatalf("GetPlatformConfiguration failed: %v", err)
	}
	cfg.Metrics.ListenAddress = address
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- StartMetricsServerWithContext(ctx) }()

	client := &http.Client{Timeout: time.Second}
	var body string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get("http://" + address + "/metrics")
		if err == nil {
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			body = string(data)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if body == "" {
		cancel()
		t.Fatal("metrics listener never became reachable")
	}
	if !strings.Contains(body, "goslow_") {
		t.Fatalf("expected goslow metrics, got:\n%s", body)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("expected a clean metrics shutdown, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("metrics server did not stop")
	}
}

func httpAuthDecisionCount(t *testing.T, outcome string) float64 {
	t.Helper()

	marker := `goslow_auth_decisions_total{outcome="` + outcome + `",transport="http"}`
	for _, line := range strings.Split(scrapeMetrics(t), "\n") {
		if !strings.HasPrefix(line, marker) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			t.Fatalf("unable to parse metric line %q: %v", line, err)
		}
		return value
	}
	return 0
}

func TestMetricsRecordHTTPAuthDecisions(t *testing.T) {
	enableTestMetrics(t)

	cfg, err := GetPlatformConfiguration()
	if err != nil {
		t.Fatalf("GetPlatformConfiguration failed: %v", err)
	}
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "metrics-http-key"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	if err := SetPlatformConfiguration(*cfg); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	router, err := newRouter(Routes{{
		Path:   "/secure",
		Method: http.MethodGet,
		Auth:   AuthLocalJWT,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	allowedBefore := httpAuthDecisionCount(t, "allowed")
	deniedBefore := httpAuthDecisionCount(t, "denied")

	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/secure", nil))
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a credential, got %d", denied.Code)
	}

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "alice"})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	allowed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/secure", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(allowed, request)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("expected an authenticated request to succeed, got %d", allowed.Code)
	}

	if got := httpAuthDecisionCount(t, "denied"); got <= deniedBefore {
		t.Fatalf("expected the denied decision to be counted, got %v (before %v)", got, deniedBefore)
	}
	if got := httpAuthDecisionCount(t, "allowed"); got <= allowedBefore {
		t.Fatalf("expected the allowed decision to be counted, got %v (before %v)", got, allowedBefore)
	}
}
