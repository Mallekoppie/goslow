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

	for index := range serviceRoutes {
		route := serviceRoutes[index]
		var handler http.Handler
		handler = route.HandlerFunc

		// Middleware is applied from the inside out.
		if route.AllowedContentType != "" {
			handler = allowedContentTypeMiddleware(handler, route.AllowedContentType)
		}

		handler = loggingMiddleware(handler, route.SlaMs)

		strategies, err := routeAuthStrategies(conf, route)
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

		if conf.HTTP.Server.AllowCorsForLocalDevelopment {
			handler = AllowCorsForLocalDevelopment(handler)

			if route.Method != http.MethodOptions {
				router.
					Path(route.Path).
					Methods(http.MethodOptions).
					Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Add("Access-Control-Allow-Origin", "*")
						w.Header().Add("Access-Control-Allow-Methods", "*")
						w.Header().Add("Access-Control-Allow-Headers", "*")
					}))
			}

		}

		router.
			Path(route.Path).
			Methods(route.Method).
			Handler(handler)
	}

	return router, nil
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

	listener, err := net.Listen("tcp", config.HTTP.Server.ListeningAddress)
	if err != nil {
		Log.Error("Unable to listen for HTTP traffic", zap.Error(err))
		return err
	}
	defer listener.Close()

	Log.Info("Starting new HTTP server", zap.String("ListeningAddress", config.HTTP.Server.ListeningAddress))

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

// Deprecated: Use StartHttpServerWithWeb
func StartHttpServerWithHtmlHosting(routes Routes, dist embed.FS) error {
	return StartHttpServerWithWeb(routes, dist)
}

func StartHttpServerWithWeb(routes Routes, dist embed.FS) error {
	router, err := newRouter(routes)
	if err != nil {
		Log.Error("Error starting HTTP server", zap.Error(err))
		return err
	}

	stripped, err := fs.Sub(dist, "dist")
	if err != nil {
		fmt.Println("Error stripping frontend")
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
	AuthRequired       bool
}

type Routes []Route
