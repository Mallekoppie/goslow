package platform

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestFailedRuntimeInitializationLeavesNoCachedConfig covers the publish-then-
// initialize regression: a configuration whose runtime bootstrap fails must not
// remain visible to later callers.
func TestFailedRuntimeInitializationLeavesNoCachedConfig(t *testing.T) {
	dir := chdirToTempDir(t)
	resetPlatformState(t)

	contents := "platform:\n  vault:\n    enabled: true\n    addresslist:\n      - \"://bad\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	bad := createDefaultConfiguration()
	bad.Vault.Enabled = true
	bad.Vault.AddressList = []string{"://bad"}

	if err := SetPlatformConfiguration(*bad); err == nil {
		t.Fatal("expected SetPlatformConfiguration to fail for an invalid Vault address")
	}
	mutex.Lock()
	published := internalConfig
	mutex.Unlock()
	if published != nil {
		t.Fatalf("expected no cached config after a failed bootstrap, got %#v", published)
	}

	if _, err := GetPlatformConfiguration(); err == nil {
		t.Fatal("expected GetPlatformConfiguration to fail while bootstrapping the same invalid config")
	}
	mutex.Lock()
	published = internalConfig
	mutex.Unlock()
	if published != nil {
		t.Fatal("expected GetPlatformConfiguration to leave no cached config after failure")
	}
}

// TestDefaultsSupplyBothListenerAddresses ensures a bare Config{} yields safe
// loopback addresses instead of empty (which would bind every interface).
func TestDefaultsSupplyBothListenerAddresses(t *testing.T) {
	resetPlatformState(t)

	if err := SetPlatformConfiguration(Config{}); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}
	conf, err := GetPlatformConfiguration()
	if err != nil {
		t.Fatalf("GetPlatformConfiguration failed: %v", err)
	}
	if conf.HTTP.Server.ListeningAddress != "127.0.0.1:10000" {
		t.Fatalf("expected the HTTP listener default, got %q", conf.HTTP.Server.ListeningAddress)
	}
	if conf.Grpc.Server.ListeningAddress != "127.0.0.1:9001" {
		t.Fatalf("expected the gRPC listener default, got %q", conf.Grpc.Server.ListeningAddress)
	}
	if err := ValidateServingConfiguration(conf); err != nil {
		t.Fatalf("expected default listeners to validate, got %v", err)
	}

	handler, err := NewApplicationHandler(ApplicationServerOptions{})
	if err != nil {
		t.Fatalf("NewApplicationHandler failed with default configuration: %v", err)
	}
	if handler == nil {
		t.Fatal("expected a handler")
	}
}

// TestConnectHandlerRequiresServiceNamesForHealth guards against enabling health
// or reflection with no services, which would register an empty checker.
func TestConnectHandlerRequiresServiceNamesForHealth(t *testing.T) {
	resetPlatformState(t)

	conf := createDefaultConfiguration()
	conf.Grpc.Server.HealthEnabled = true
	if err := SetPlatformConfiguration(*conf); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	if _, err := NewConnectHandler(ConnectServerOptions{}); err == nil {
		t.Fatal("expected an error when health is enabled without service names")
	}

	handler, err := NewConnectHandler(ConnectServerOptions{ServiceNames: []string{"example.v1.Service"}})
	if err != nil {
		t.Fatalf("expected health registration to succeed with service names: %v", err)
	}
	if handler == nil {
		t.Fatal("expected a handler")
	}

	conf.Grpc.Server.HealthEnabled = false
	conf.Grpc.Server.ReflectionEnabled = false
	if err := SetPlatformConfiguration(*conf); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}
	if _, err := NewConnectHandler(ConnectServerOptions{}); err != nil {
		t.Fatalf("expected no error when health and reflection are disabled: %v", err)
	}
}

// TestMetricsLifecycleFollowsConfiguration proves that disabling metrics stops
// recording and re-enabling resumes into the same registry.
func TestMetricsLifecycleFollowsConfiguration(t *testing.T) {
	resetPlatformState(t)

	conf := createDefaultConfiguration()
	conf.Metrics.Enabled = true
	conf.Metrics.ListenAddress = "127.0.0.1:0"
	if err := SetPlatformConfiguration(*conf); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}
	if !metricsEnabled() {
		t.Fatal("expected metrics to be active")
	}

	conf.Metrics.Enabled = false
	if err := SetPlatformConfiguration(*conf); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}
	if metricsEnabled() {
		t.Fatal("expected metrics recording to stop when disabled")
	}

	conf.Metrics.Enabled = true
	if err := SetPlatformConfiguration(*conf); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}
	if !metricsEnabled() {
		t.Fatal("expected metrics to resume after being re-enabled")
	}
	if metricsRegistry == nil {
		t.Fatal("expected the registry to be retained for the process lifetime")
	}
}

// TestMetricsListenerStartsOnce covers the double-bind guard in
// startMetricsServerIfEnabled.
func TestMetricsListenerStartsOnce(t *testing.T) {
	resetPlatformState(t)

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	address := probe.Addr().String()
	probe.Close()

	conf := createDefaultConfiguration()
	conf.Metrics.Enabled = true
	conf.Metrics.ListenAddress = address
	if err := SetPlatformConfiguration(*conf); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	startMetricsServerIfEnabled(ctx, conf)
	startMetricsServerIfEnabled(ctx, conf)

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	reachable := false
	for time.Now().Before(deadline) {
		response, err := client.Get("http://" + address + "/metrics")
		if err == nil {
			response.Body.Close()
			reachable = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !reachable {
		cancel()
		t.Fatal("metrics listener never became reachable")
	}

	metricsListenerLock.Lock()
	running := metricsListenerRunning
	metricsListenerLock.Unlock()
	if !running {
		t.Fatal("expected exactly one metrics listener to be running")
	}

	cancel()
	stopDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(stopDeadline) {
		metricsListenerLock.Lock()
		running = metricsListenerRunning
		metricsListenerLock.Unlock()
		if !running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("metrics listener did not stop after cancellation")
}

// TestLocalJWTValidationFailuresDoNotLogErrors ensures a verifier miss in a
// multi-strategy setup is not reported at error level.
func TestLocalJWTValidationFailuresDoNotLogErrors(t *testing.T) {
	resetPlatformState(t)

	conf := createDefaultConfiguration()
	conf.Log.FileLoggingEnabled = false
	conf.Auth.Server.LocalJwt.Enabled = true
	conf.Auth.Server.LocalJwt.JwtSigningKey = "test-signing-key"
	conf.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	if err := SetPlatformConfiguration(*conf); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	var buffer bytes.Buffer
	previous := Log.current.Load()
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&buffer), zap.ErrorLevel)
	Log.current.Store(zap.New(core))
	t.Cleanup(func() {
		Log.current.Store(previous)
	})

	if _, err := LocalJwt.ValidateLocalJwtToken("not-a-jwt"); err == nil {
		t.Fatal("expected an invalid token error")
	}
	expired, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"exp": time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}
	if _, err := LocalJwt.ValidateLocalJwtToken(expired); err == nil {
		t.Fatal("expected an expired token error")
	}

	if buffer.Len() != 0 {
		t.Fatalf("expected no error-level log entries, got:\n%s", buffer.String())
	}
}

// TestOIDCSuccessDoesNotLogLocalJWTErrors covers the AuthLocalOrOIDC order:
// the local verifier misses first, then OIDC succeeds, without error logging.
func TestOIDCSuccessDoesNotLogLocalJWTErrors(t *testing.T) {
	resetPlatformState(t)

	conf := createDefaultConfiguration()
	conf.Auth.Server.LocalJwt.Enabled = true
	conf.Auth.Server.LocalJwt.JwtSigningKey = "test-signing-key"
	conf.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	if err := SetPlatformConfiguration(*conf); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	var buffer bytes.Buffer
	previous := Log.current.Load()
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&buffer), zap.ErrorLevel)
	Log.current.Store(zap.New(core))
	t.Cleanup(func() {
		Log.current.Store(previous)
	})

	oidcSuccess := func(r *http.Request) (authResult, error) {
		return authResult{Username: "oidc-user"}, nil
	}
	handler := anyAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), localJWTVerifier(), oidcSuccess)

	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	request.Header.Set("Authorization", "Bearer oidc-issued-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected the OIDC verifier to authenticate, got %d", response.Code)
	}
	if buffer.Len() != 0 {
		t.Fatalf("expected no error-level log entries, got:\n%s", buffer.String())
	}
}

// TestHTTPRoutesAreWriteBounded covers the combined-server fix: an HTTP route
// that exceeds the write budget returns 503 while Connect streams are exempt.
func TestHTTPRoutesAreWriteBounded(t *testing.T) {
	t.Run("middleware", func(t *testing.T) {
		handler := boundHTTPRouteWrites(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}), 20*time.Millisecond)

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/slow", nil))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 for a timed-out route, got %d", response.Code)
		}
	})

	t.Run("application handler", func(t *testing.T) {
		resetPlatformState(t)

		conf := createDefaultConfiguration()
		conf.HTTP.Server.WriteTimeoutSeconds = 1
		if err := SetPlatformConfiguration(*conf); err != nil {
			t.Fatalf("SetPlatformConfiguration failed: %v", err)
		}

		handler, err := NewApplicationHandler(ApplicationServerOptions{
			Routes: Routes{{
				Path:   "/slow",
				Method: http.MethodGet,
				Auth:   AuthNone,
				HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(1500 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
				},
			}},
		})
		if err != nil {
			t.Fatalf("NewApplicationHandler failed: %v", err)
		}

		server := httptest.NewServer(handler)
		defer server.Close()

		start := time.Now()
		response, err := server.Client().Get(server.URL + "/slow")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", response.StatusCode)
		}
		if elapsed := time.Since(start); elapsed > 1400*time.Millisecond {
			t.Fatalf("expected the route to be abandoned near the budget, took %s", elapsed)
		}

		serverConf := newApplicationHTTPServer(conf, handler)
		if serverConf.WriteTimeout != 0 {
			t.Fatalf("expected streaming writes to stay unbounded at the server layer, got %s", serverConf.WriteTimeout)
		}
	})
}

// TestBoundHTTPRouteWritesDisabled documents that a non-positive budget keeps
// the handler unwrapped.
func TestBoundHTTPRouteWritesDisabled(t *testing.T) {
	called := false
	handler := boundHTTPRouteWrites(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}), 0)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatal("expected the handler to run")
	}
}
