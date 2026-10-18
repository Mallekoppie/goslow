package platform

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const httpShutdownTimeout = 10 * time.Second

func newRouter(serviceRoutes Routes) (*mux.Router, error) {
	router := mux.NewRouter().StrictSlash(true)

	conf, err := GetPlatformConfiguration()
	if err != nil {
		return nil, err
	}

	// Request ID runs first so the recovery handler can report it.
	router.Use(requestIDMiddleware, recoveryMiddleware)

	matcher, err := publicPaths(conf)
	if err != nil {
		return nil, err
	}

	for index := range serviceRoutes {
		route := serviceRoutes[index]
		var handler http.Handler
		handler = route.HandlerFunc

		// Middleware is applied from the inside out.
		if route.AllowedContentType != "" {
			handler = allowedContentTypeMiddleware(handler, route.AllowedContentType)
		}

		excluded := matcher.matches(route.Method, route.Path)
		if excluded {
			if route.Auth.Enabled() {
				return nil, fmt.Errorf("route %s %s declares auth %q but is also in the unauthenticated exclusion list", route.Method, route.Path, route.Auth)
			}
			route.Auth = AuthNone
		}

		strategies, err := routeAuthStrategies(route)
		if err != nil {
			return nil, err
		}

		verifiers := make([]authVerifier, 0, len(strategies))
		for _, strategy := range strategies {
			verifier, err := authStrategyVerifier(conf, strategy, route.RolesRequired)
			if err != nil {
				return nil, err
			}
			verifiers = append(verifiers, verifier)
		}

		if len(verifiers) > 0 {
			if route.Auth == AuthLocalOrOIDC {
				handler = anyAuthMiddleware(handler, verifiers...)
			} else {
				handler = allAuthMiddleware(handler, verifiers...)
			}
		}

		// Logging wraps auth so 401/403 responses are logged and counted.
		handler = loggingMiddleware(handler, route.Path, route.SlaMs)

		// HTTP routes are bounded by http.server.writetimeoutseconds. Connect
		// RPCs bypass this middleware, so streaming responses stay open.
		handler = boundHTTPRouteWrites(handler, seconds(conf.HTTP.Server.WriteTimeoutSeconds))

		if len(conf.HTTP.Server.AllowedOrigins) > 0 {
			handler = corsMiddleware(handler, conf.HTTP.Server.AllowedOrigins, false)
			if route.Method != http.MethodOptions {
				router.
					Path(route.Path).
					Methods(http.MethodOptions).
					Handler(corsMiddleware(http.NotFoundHandler(), conf.HTTP.Server.AllowedOrigins, false))
			}
		}

		router.
			Path(route.Path).
			Methods(route.Method).
			Handler(handler)
	}

	return router, nil
}

// boundHTTPRouteWrites caps how long a single HTTP route may take to produce
// its response. When the deadline elapses the caller receives 503 and the
// handler is abandoned. Non-positive durations disable the bound.
func boundHTTPRouteWrites(next http.Handler, timeout time.Duration) http.Handler {
	if timeout <= 0 {
		return next
	}
	return http.TimeoutHandler(next, timeout, `{"error":"request timeout"}`)
}

// newHTTPServer builds the server with the configured timeouts.
func newHTTPServer(conf *Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              conf.HTTP.Server.ListeningAddress,
		Handler:           handler,
		ReadHeaderTimeout: time.Duration(conf.HTTP.Server.ReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout:       time.Duration(conf.HTTP.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(conf.HTTP.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(conf.HTTP.Server.IdleTimeoutSeconds) * time.Second,
	}
}

// serveHTTPServer serves until the context is cancelled, then shuts down
// gracefully. http.ErrServerClosed is treated as a clean exit.
func serveHTTPServer(ctx context.Context, server *http.Server, listener net.Listener, tlsEnabled bool, certFile, keyFile string) error {
	serveErrors := make(chan error, 1)
	go func() {
		var err error
		if tlsEnabled {
			err = server.ServeTLS(listener, certFile, keyFile)
		} else {
			err = server.Serve(listener)
		}
		serveErrors <- err
	}()

	select {
	case err := <-serveErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			Log.Error("HTTP server stopped unexpectedly", zap.Error(err))
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			Log.Error("HTTP server shutdown failed", zap.Error(err))
			return err
		}

		if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			Log.Error("HTTP server stopped with error during shutdown", zap.Error(err))
			return err
		}
		return nil
	}
}

func startHttpServerInternal(router *mux.Router) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return serveRoutes(ctx, router)
}

func serveRoutes(ctx context.Context, router *mux.Router) error {
	config, err := GetPlatformConfiguration()
	if err != nil {
		Log.Error("Error reading platform configuration", zap.Error(err))
		return err
	}

	if err := ValidateServingConfiguration(config); err != nil {
		Log.Error("Invalid serving configuration", zap.Error(err))
		return err
	}

	listener, err := net.Listen("tcp", config.HTTP.Server.ListeningAddress)
	if err != nil {
		Log.Error("Unable to listen for HTTP traffic", zap.Error(err))
		return err
	}
	defer listener.Close()

	Log.Info("Starting new HTTP server", zap.String("ListeningAddress", config.HTTP.Server.ListeningAddress))

	startMetricsServerIfEnabled(ctx, config)

	return serveHTTPServer(ctx, newHTTPServer(config, router), listener,
		config.HTTP.Server.TLSEnabled, config.HTTP.Server.TLSCertFileName, config.HTTP.Server.TLSKeyFileName)
}

func StartHttpServer(routes Routes) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return StartHttpServerWithContext(ctx, routes)
}

// StartHttpServerWithContext starts the HTTP server and shuts it down gracefully
// when the context is cancelled.
func StartHttpServerWithContext(ctx context.Context, routes Routes) error {
	router, err := newRouter(routes)
	if err != nil {
		Log.Error("Error starting HTTP server", zap.Error(err))
		return err
	}

	return serveRoutes(ctx, router)
}

func StartHttpServerWithWeb(routes Routes, dist embed.FS) error {
	router, err := newRouter(routes)
	if err != nil {
		Log.Error("Error starting HTTP server", zap.Error(err))
		return err
	}

	stripped, err := fs.Sub(dist, "dist")
	if err != nil {
		Log.Error("Error stripping frontend", zap.Error(err))
		return err
	}
	fileServer := http.FileServer(http.FS(stripped))
	router.PathPrefix("/").Handler(fileServer)

	return startHttpServerInternal(router)
}

type Route struct {
	Path               string
	Method             string
	HandlerFunc        http.HandlerFunc
	SlaMs              int64
	RolesRequired      []string
	AllowedContentType string
	Auth               AuthStrategy
}

type Routes []Route
