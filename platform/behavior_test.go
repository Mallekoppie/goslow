package platform

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartHttpServerWithContextServesUntilCancelled(t *testing.T) {
	cfg := setupTestPlatformState(t)

	// Reserve a port, then hand it to the server.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	address := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}

	cfg.HTTP.Server.ListeningAddress = address
	SetPlatformConfiguration(*cfg)

	ctx, cancel := context.WithCancel(context.Background())
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- StartHttpServerWithContext(ctx, Routes{{
			Path:   "/ping",
			Method: http.MethodGet,
			HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("pong"))
			},
		}})
	}()

	var response *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err = http.Get("http://" + address + "/ping")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("server never became reachable: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}

	cancel()
	select {
	case err := <-serverErrors:
		if err != nil {
			t.Fatalf("expected a clean shutdown, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func chdirToTempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	return dir
}

func TestGetPlatformConfigurationReadsFileAndReportsErrors(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		dir := chdirToTempDir(t)
		resetPlatformState(t)

		if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("platform:\n  log:\n    level: debug\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		cfg, err := GetPlatformConfiguration()
		if err != nil {
			t.Fatalf("GetPlatformConfiguration failed: %v", err)
		}
		if cfg.Log.Level != "debug" {
			t.Fatalf("expected the file value, got %q", cfg.Log.Level)
		}
	})

	t.Run("invalid file", func(t *testing.T) {
		dir := chdirToTempDir(t)
		resetPlatformState(t)

		if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("platform:\n  log:\n    level: [unclosed\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if _, err := GetPlatformConfiguration(); err == nil {
			t.Fatal("expected a parse error")
		}
	})
}

func TestCheckPlatformConfigurationRejectsNil(t *testing.T) {
	var conf *Config
	if err := conf.checkPlatformConfiguration(); err == nil {
		t.Fatal("expected an error for a nil config")
	}
}

func TestSavePlatformConfigurationWritesLoadedFile(t *testing.T) {
	dir := chdirToTempDir(t)
	resetPlatformState(t)

	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("platform:\n  log:\n    level: debug\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := GetPlatformConfiguration(); err != nil {
		t.Fatalf("load config: %v", err)
	}

	conf := createDefaultConfiguration()
	conf.Component.ComponentName = "saved"
	if err := SavePlatformConfiguration(*conf); err != nil {
		t.Fatalf("SavePlatformConfiguration failed: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(contents), "saved") {
		t.Fatalf("expected the saved component name in %s", contents)
	}
}

func TestMarshallingErrorBranches(t *testing.T) {
	setupTestPlatformState(t)

	if err := JsonMarshaller.ReadJsonRequestLimit(nil, nil, 10); err != nil {
		t.Fatalf("nil body should be a no-op, got %v", err)
	}

	var payload map[string]interface{}
	if err := JsonMarshaller.ReadJsonRequest(readCloserFrom(`{"a":1}`), &payload); err != nil {
		t.Fatalf("expected a valid body to decode: %v", err)
	}
	if payload["a"].(float64) != 1 {
		t.Fatalf("unexpected payload: %v", payload)
	}

	body := readCloserFrom(`{"unknown":true}`)
	var target struct {
		Name string `json:"name"`
	}
	if err := JsonMarshaller.ReadJsonRequest(body, &target); err == nil {
		t.Fatal("expected unknown fields to be rejected")
	}

	body = readCloserFrom(`{`)
	if err := JsonMarshaller.ReadJsonRequest(body, &target); err == nil {
		t.Fatal("expected invalid json to be rejected")
	}

	// Unmarshalable values produce a 500 instead of a panic.
	response := httptest.NewRecorder()
	JsonMarshaller.WriteJsonResponse(response, http.StatusOK, make(chan int))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for an unmarshalable value, got %d", response.Code)
	}

	// A nil writer is ignored.
	JsonMarshaller.WriteJsonResponse(nil, http.StatusOK, map[string]string{"a": "b"})

	// Status 0 leaves the default 200.
	response = httptest.NewRecorder()
	JsonMarshaller.WriteJsonResponse(response, 0, map[string]string{"a": "b"})
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}

func TestBoltDBErrorBranches(t *testing.T) {
	withTestDB(t)

	if err := Database.BoltDb.ReadObject("bucket", "key", nil); err == nil {
		t.Fatal("expected an error for a nil target")
	}

	if err := Database.BoltDb.ReadObject("missing", "key", &struct{}{}); !errors.Is(err, ErrNoEntryFoundInDB) {
		t.Fatalf("expected ErrNoEntryFoundInDB for a missing bucket, got %v", err)
	}

	if err := Database.BoltDb.SaveObject("test", "unmarshalable", make(chan int)); err == nil {
		t.Fatal("expected a marshal error")
	}

	if err := Database.BoltDb.SaveObjects("test", nil); err != nil {
		t.Fatalf("an empty batch should be a no-op, got %v", err)
	}

	// Removing something that is absent is not an error.
	if err := Database.BoltDb.RemoveObject("test", "absent"); err != nil {
		t.Fatalf("RemoveObject failed: %v", err)
	}
	if err := Database.BoltDb.RemoveBucket("absent-bucket"); err != nil {
		t.Fatalf("RemoveBucket should ignore a missing bucket, got %v", err)
	}
	if err := Database.BoltDb.RemoveBucket("test"); err != nil {
		t.Fatalf("RemoveBucket failed: %v", err)
	}

	if err := Database.BoltDb.Scan("missing", func(key, value []byte) error { return nil }); !errors.Is(err, ErrNoEntryFoundInDB) {
		t.Fatalf("expected ErrNoEntryFoundInDB from Scan, got %v", err)
	}
	if err := Database.BoltDb.Scan("missing", nil); err == nil {
		t.Fatal("expected an error for a nil scan function")
	}
	if err := Database.BoltDb.ScanPrefix("missing", "p", func(key, value []byte) error { return nil }); !errors.Is(err, ErrNoEntryFoundInDB) {
		t.Fatalf("expected ErrNoEntryFoundInDB from ScanPrefix, got %v", err)
	}
	if err := Database.BoltDb.ScanPrefix("missing", "p", nil); err == nil {
		t.Fatal("expected an error for a nil prefix scan function")
	}
	if err := DeleteDatabase(""); err == nil {
		t.Fatal("expected an error for an empty delete path")
	}
}

func TestBoltDBDisabledOperationsFail(t *testing.T) {
	resetPlatformState(t)
	conf := createDefaultConfiguration()
	conf.Database.BoltDB.Enabled = false
	SetPlatformConfiguration(*conf)

	if err := Database.BoltDb.SaveObject("b", "k", map[string]string{"a": "b"}); !errors.Is(err, ErrBoltDbIsNotEnabled) {
		t.Fatalf("expected ErrBoltDbIsNotEnabled, got %v", err)
	}
	if _, err := Database.BoltDb.Count("b"); !errors.Is(err, ErrBoltDbIsNotEnabled) {
		t.Fatalf("expected ErrBoltDbIsNotEnabled, got %v", err)
	}
	if _, err := Database.BoltDb.Stats(); !errors.Is(err, ErrBoltDbIsNotEnabled) {
		t.Fatalf("expected ErrBoltDbIsNotEnabled, got %v", err)
	}
}

func readCloserFrom(value string) *closingReader {
	return &closingReader{reader: strings.NewReader(value)}
}

type closingReader struct {
	reader *strings.Reader
}

func (c *closingReader) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *closingReader) Close() error               { return nil }
