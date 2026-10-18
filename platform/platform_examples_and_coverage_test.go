package platform

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestPlatformState(t *testing.T) *Config {
	t.Helper()
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Component.ComponentName = "unit-test"
	cfg.Log.Level = "info"
	cfg.Log.FileLoggingEnabled = false
	cfg.HTTP.Clients = []httpClientConfig{{
		ID:                 "default",
		TLSVerify:          true,
		MaxIdleConnections: 10,
		RequestTimeout:     2,
	}}
	cfg.Database.BoltDB.Enabled = true
	cfg.Database.BoltDB.FileName = filepath.Join(t.TempDir(), "platform.db")
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "unit-test-signing-key"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"

	SetPlatformConfiguration(*cfg)
	return cfg
}

func TestSetAndGetPlatformConfiguration(t *testing.T) {
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Component.ComponentName = "config-demo"
	SetPlatformConfiguration(*cfg)

	actual, err := GetPlatformConfiguration()
	if err != nil {
		t.Fatalf("GetPlatformConfiguration returned error: %v", err)
	}
	if actual == nil {
		t.Fatal("GetPlatformConfiguration returned nil config")
	}
	if actual.Component.ComponentName != "config-demo" {
		t.Fatalf("expected component name %q, got %q", "config-demo", actual.Component.ComponentName)
	}
}

func TestConfigureLoggerWithConfigAndLevelFallback(t *testing.T) {
	resetPlatformState(t)
	cfg := createDefaultConfiguration()
	cfg.Log.Level = "debug"
	ConfigureLoggerWithConfig(cfg)
	if Log == nil {
		t.Fatal("Log should not be nil after configure")
	}

	_, err := logLevelStringToZapType("not-a-level")
	if !errors.Is(err, ErrInvalidLogLevel) {
		t.Fatalf("expected ErrInvalidLogLevel for invalid log level, got %v", err)
	}

	level, err := logLevelStringToZapType("warn")
	if err != nil {
		t.Fatalf("warn should be valid: %v", err)
	}
	if level.String() != "warn" {
		t.Fatalf("expected warn level, got %q", level.String())
	}
}

func TestCreateHttpClient(t *testing.T) {
	setupTestPlatformState(t)

	client, err := CreateHttpClient("default")
	if err != nil {
		t.Fatalf("CreateHttpClient returned error: %v", err)
	}
	if client == nil {
		t.Fatal("CreateHttpClient returned nil client")
	}
	if client.Timeout != 2*time.Second {
		t.Fatalf("expected timeout 2s, got %s", client.Timeout)
	}

	_, err = CreateHttpClient("missing")
	if !errors.Is(err, ErrHttpClientConfigNotFound) {
		t.Fatalf("expected ErrHttpClientConfigNotFound, got %v", err)
	}
}

func TestJsonMarshallerReadAndWrite(t *testing.T) {
	setupTestPlatformState(t)

	body := io.NopCloser(strings.NewReader(`{"name":"Ada"}`))
	var payload struct {
		Name string `json:"name"`
	}
	if err := JsonMarshaller.ReadJsonRequest(body, &payload); err != nil {
		t.Fatalf("ReadJsonRequest failed: %v", err)
	}
	if payload.Name != "Ada" {
		t.Fatalf("expected payload name Ada, got %q", payload.Name)
	}

	largeBody := io.NopCloser(strings.NewReader(strings.Repeat("A", int(DefaultReadJsonRequestLimit)+1)))
	if err := JsonMarshaller.ReadJsonRequestLimit(largeBody, &payload, DefaultReadJsonRequestLimit); !errors.Is(err, ErrRequestBodyTooLarge) {
		t.Fatalf("expected ErrRequestBodyTooLarge, got %v", err)
	}

	recorder := httptest.NewRecorder()
	JsonMarshaller.WriteJsonResponse(recorder, http.StatusCreated, map[string]string{"status": "ok"})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status code %d, got %d", http.StatusCreated, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "\"status\":\"ok\"") {
		t.Fatalf("unexpected response body: %s", recorder.Body.String())
	}
}

func TestBearerTokenAndLocalJwtMiddleware(t *testing.T) {
	setupTestPlatformState(t)

	if _, ok := bearerToken(""); ok {
		t.Fatal("empty bearer token should be rejected")
	}
	if _, ok := bearerToken("abc"); ok {
		t.Fatal("non-Bearer tokens should be rejected")
	}
	if _, ok := bearerToken("Basic xyz"); ok {
		t.Fatal("basic auth should be rejected")
	}
	if _, ok := bearerToken("Bearer "); ok {
		t.Fatal("empty bearer value should be rejected")
	}

	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "alice"})
	if err != nil {
		t.Fatalf("NewLocalJwtToken failed: %v", err)
	}

	handler := localJwtAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	for _, tc := range []struct {
		header string
		want   int
	}{
		{header: "", want: http.StatusUnauthorized},
		{header: "Basic xyz", want: http.StatusUnauthorized},
		{header: "Bearer ", want: http.StatusUnauthorized},
		{header: "Bearer " + token, want: http.StatusAccepted},
	} {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.want {
			t.Fatalf("for header %q, expected status %d, got %d", tc.header, tc.want, res.Code)
		}
	}
}

func TestBoltDBRoundTripAndCleanup(t *testing.T) {
	path := withTestDB(t)

	testItem := struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}{
		ID:   "123",
		Name: "Ada",
	}
	if err := Database.BoltDb.SaveObject("people", testItem.ID, testItem); err != nil {
		t.Fatalf("SaveObject failed: %v", err)
	}

	var stored struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := Database.BoltDb.ReadObject("people", testItem.ID, &stored); err != nil {
		t.Fatalf("ReadObject failed: %v", err)
	}
	if stored.Name != testItem.Name {
		t.Fatalf("expected name %q, got %q", testItem.Name, stored.Name)
	}

	if _, err := Database.BoltDb.ReadAllObjects("people"); err != nil {
		t.Fatalf("ReadAllObjects failed: %v", err)
	}

	if err := Database.BoltDb.RemoveObject("people", testItem.ID); err != nil {
		t.Fatalf("RemoveObject failed: %v", err)
	}
	if err := Database.BoltDb.RemoveBucket("people"); err != nil {
		t.Fatalf("RemoveBucket failed: %v", err)
	}
	if err := DeleteDatabase(path); err != nil {
		t.Fatalf("DeleteDatabase failed: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected db file to be removed from %q, stat err=%v", path, statErr)
	}
}

func ExampleCreateHttpClient() {
	cfg := createDefaultConfiguration()
	cfg.HTTP.Clients = []httpClientConfig{{ID: "default", TLSVerify: true, MaxIdleConnections: 2, RequestTimeout: 2}}
	SetPlatformConfiguration(*cfg)
	client, err := CreateHttpClient("default")
	if err != nil {
		panic(err)
	}
	fmt.Println(client.Timeout)
	// Output: 2s
}

func ExampleSetPlatformConfiguration() {
	cfg := createDefaultConfiguration()
	cfg.Component.ComponentName = "demo-component"
	SetPlatformConfiguration(*cfg)
	fmt.Println(cfg.Component.ComponentName)
	// Output: demo-component
}

func ExampleGetPlatformConfiguration() {
	cfg := createDefaultConfiguration()
	cfg.Component.ComponentName = "loaded-component"
	SetPlatformConfiguration(*cfg)
	current, err := GetPlatformConfiguration()
	if err != nil {
		panic(err)
	}
	fmt.Println(current.Component.ComponentName)
	// Output: loaded-component
}

func Example_readJsonRequest() {
	body := io.NopCloser(strings.NewReader(`{"name":"Ada"}`))
	var payload struct {
		Name string `json:"name"`
	}
	if err := JsonMarshaller.ReadJsonRequest(body, &payload); err != nil {
		panic(err)
	}
	fmt.Println(payload.Name)
	// Output: Ada
}

func Example_writeJsonResponse() {
	recorder := httptest.NewRecorder()
	JsonMarshaller.WriteJsonResponse(recorder, http.StatusCreated, map[string]string{"status": "ok"})
	fmt.Println(recorder.Code)
	fmt.Println(recorder.Body.String())
	// Output:
	// 201
	// {"status":"ok"}
}

func Example_newLocalJwtToken() {
	cfg := createDefaultConfiguration()
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "demo-key"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	SetPlatformConfiguration(*cfg)
	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "alice"})
	if err != nil {
		panic(err)
	}
	fmt.Println(strings.Contains(token, "."))
	// Output: true
}

func Example_validateLocalJwtToken() {
	cfg := createDefaultConfiguration()
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "demo-key"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	SetPlatformConfiguration(*cfg)
	token, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "alice"})
	if err != nil {
		panic(err)
	}
	claims, err := LocalJwt.ValidateLocalJwtToken(token)
	if err != nil {
		panic(err)
	}
	fmt.Println(claims["sub"])
	// Output: alice
}

func ExampleSetupBoltDB() {
	cfg := createDefaultConfiguration()
	cfg.Database.BoltDB.Enabled = true
	cfg.Database.BoltDB.FileName = filepath.Join(os.TempDir(), "goslow-example.db")
	SetPlatformConfiguration(*cfg)
	if err := SetupBoltDB(); err != nil {
		panic(err)
	}
	defer func() { _ = Database.BoltDb.Close(); _ = os.Remove(cfg.Database.BoltDB.FileName) }()
	fmt.Println(dbBolt != nil)
	// Output: true
}
