package platform

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/mux"
)

// ApplicationServerOptions mounts Connect, native gRPC, gRPC-Web, optional
// platform HTTP routes, and optional web/SPA assets on one origin.
type ApplicationServerOptions struct {
	Services              []ConnectService
	ServiceNames          []string
	HandlerOptions        []connect.HandlerOption
	Interceptors          []connect.Interceptor
	DisableAuthFromConfig bool

	Routes     Routes
	WebHandler http.Handler
	SPA        *SPAOptions

	AllowedOrigins  []string
	TLSEnabled      bool
	TLSCertFileName string
	TLSKeyFileName  string
}

// NewApplicationHandler composes the RPC handler, HTTP routes, and web assets.
// RPC-shaped requests never fall through to routes or assets.
func NewApplicationHandler(options ApplicationServerOptions) (http.Handler, error) {
	conf, err := GetPlatformConfiguration()
	if err != nil {
		return nil, err
	}
	if conf == nil {
		conf = createDefaultConfiguration()
	}
	initializeMetrics(conf)

	connectHandler, err := NewConnectHandler(ConnectServerOptions{
		Services:              options.Services,
		ServiceNames:          options.ServiceNames,
		HandlerOptions:        options.HandlerOptions,
		Interceptors:          options.Interceptors,
		DisableAuthFromConfig: options.DisableAuthFromConfig,
		AllowedOrigins:        options.AllowedOrigins,
		TLSEnabled:            options.TLSEnabled,
		TLSCertFileName:       options.TLSCertFileName,
		TLSKeyFileName:        options.TLSKeyFileName,
	})
	if err != nil {
		return nil, err
	}

	httpRouter, err := newRouter(options.Routes)
	if err != nil {
		return nil, err
	}

	var webHandler http.Handler
	if options.SPA != nil {
		webHandler = NewSPAHandler(options.SPA.Assets, options.SPA.Index)
	} else {
		webHandler = options.WebHandler
	}

	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRPCRequest(r) {
			connectHandler.ServeHTTP(w, r)
			return
		}

		match := &mux.RouteMatch{}
		if httpRouter.Match(r, match) {
			httpRouter.ServeHTTP(w, r)
			return
		}

		if webHandler != nil {
			webHandler.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})

	return requestIDMiddleware(recoveryMiddleware(dispatch)), nil
}

// ServeApplication serves a combined application on a caller-owned listener
// until the context is cancelled.
func ServeApplication(ctx context.Context, listener net.Listener, options ApplicationServerOptions) error {
	if listener == nil {
		return errors.New("nil application listener")
	}

	conf, err := GetPlatformConfiguration()
	if err != nil {
		return err
	}
	if conf == nil {
		conf = createDefaultConfiguration()
	}
	if err := ValidateServingConfiguration(conf); err != nil {
		return err
	}

	handler, err := NewApplicationHandler(options)
	if err != nil {
		return err
	}

	return serveHTTPServer(ctx, newApplicationHTTPServer(conf, handler), listener,
		options.TLSEnabled, options.TLSCertFileName, options.TLSKeyFileName)
}

// StartApplicationServerWithContext listens on the HTTP address and serves the
// combined application until the context is cancelled.
func StartApplicationServerWithContext(ctx context.Context, options ApplicationServerOptions) error {
	conf, err := GetPlatformConfiguration()
	if err != nil {
		return err
	}
	if conf == nil {
		conf = createDefaultConfiguration()
	}

	options.TLSEnabled = conf.HTTP.Server.TLSEnabled || conf.Grpc.Server.TLSEnabled
	if options.TLSEnabled {
		if conf.HTTP.Server.TLSCertFileName != "" {
			options.TLSCertFileName = conf.HTTP.Server.TLSCertFileName
			options.TLSKeyFileName = conf.HTTP.Server.TLSKeyFileName
		} else {
			options.TLSCertFileName = conf.Grpc.Server.TLSCertFileName
			options.TLSKeyFileName = conf.Grpc.Server.TLSKeyFileName
		}
	}

	address := conf.HTTP.Server.ListeningAddress
	if address == "" {
		address = conf.Grpc.Server.ListeningAddress
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()

	startMetricsServerIfEnabled(ctx, conf)

	return ServeApplication(ctx, listener, options)
}

func newApplicationHTTPServer(conf *Config, handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: seconds(conf.HTTP.Server.ReadHeaderTimeoutSeconds),
		ReadTimeout:       seconds(conf.HTTP.Server.ReadTimeoutSeconds),
		// Unary RPCs and HTTP routes are bounded per request; streams are not.
		WriteTimeout: 0,
		IdleTimeout:  seconds(conf.HTTP.Server.IdleTimeoutSeconds),
	}
}

func seconds(value int) time.Duration {
	return time.Duration(value) * time.Second
}
