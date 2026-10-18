package platform

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServerUsesConfiguredTimeouts(t *testing.T) {
	conf := createDefaultConfiguration()
	conf.HTTP.Server.ListeningAddress = "127.0.0.1:12345"
	conf.HTTP.Server.ReadHeaderTimeoutSeconds = 3
	conf.HTTP.Server.ReadTimeoutSeconds = 4
	conf.HTTP.Server.WriteTimeoutSeconds = 5
	conf.HTTP.Server.IdleTimeoutSeconds = 6

	server := newHTTPServer(conf, http.NewServeMux())
	if server.Addr != "127.0.0.1:12345" {
		t.Fatalf("unexpected address: %q", server.Addr)
	}
	if server.ReadHeaderTimeout != 3*time.Second {
		t.Fatalf("expected 3s read header timeout, got %s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != 4*time.Second {
		t.Fatalf("expected 4s read timeout, got %s", server.ReadTimeout)
	}
	if server.WriteTimeout != 5*time.Second {
		t.Fatalf("expected 5s write timeout, got %s", server.WriteTimeout)
	}
	if server.IdleTimeout != 6*time.Second {
		t.Fatalf("expected 6s idle timeout, got %s", server.IdleTimeout)
	}
}

func TestConfigurationDefaultsHTTPAndDatabaseTimeouts(t *testing.T) {
	conf := createDefaultConfiguration()
	if conf.Database.BoltDB.OpenTimeoutSeconds != 5 {
		t.Fatalf("expected a 5s bolt open timeout, got %d", conf.Database.BoltDB.OpenTimeoutSeconds)
	}
	if conf.HTTP.Server.ReadHeaderTimeoutSeconds <= 0 || conf.HTTP.Server.ReadTimeoutSeconds <= 0 ||
		conf.HTTP.Server.WriteTimeoutSeconds <= 0 || conf.HTTP.Server.IdleTimeoutSeconds <= 0 {
		t.Fatalf("expected non-zero HTTP server timeouts, got %+v", conf.HTTP.Server)
	}
	if conf.Auth.Server.OAuth.RolesClaim != "roles" {
		t.Fatalf("expected the default roles claim to be \"roles\", got %q", conf.Auth.Server.OAuth.RolesClaim)
	}
}

func TestServeHTTPServerShutsDownGracefully(t *testing.T) {
	conf := createDefaultConfiguration()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	router, err := newRouter(Routes{{
		Path:   "/ping",
		Method: http.MethodGet,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("pong"))
		},
	}})
	if err != nil {
		t.Fatalf("newRouter failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serveHTTPServer(ctx, newHTTPServer(conf, router), listener, false, "", "")
	}()

	response, err := http.Get("http://" + listener.Addr().String() + "/ping")
	if err != nil {
		cancel()
		t.Fatalf("request failed: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		cancel()
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "pong" {
		cancel()
		t.Fatalf("expected pong, got %q", body)
	}

	cancel()
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("expected a clean shutdown, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within the timeout")
	}
}

func TestServeHTTPServerRejectsClosedListener(t *testing.T) {
	conf := createDefaultConfiguration()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	server := newHTTPServer(conf, http.NewServeMux())
	err = serveHTTPServer(context.Background(), server, listener, false, "", "")
	if err == nil {
		t.Fatal("expected an error when the listener is already closed")
	}
}
