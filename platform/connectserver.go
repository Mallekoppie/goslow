package platform

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	"go.uber.org/zap"
)

// ConnectService is implemented by generated Connect service handlers. A
// generated handler is registered on the supplied mux; the platform never owns
// the service implementation.
//
// Connect speaks the Connect, gRPC, and gRPC-Web protocols, so services
// registered here are gRPC compatible without importing google.golang.org/grpc.
type ConnectService interface {
	Register(mux *http.ServeMux, options ...connect.HandlerOption)
}

// ConnectServerOptions configures the HTTP handler that fronts Connect, gRPC,
// and gRPC-Web traffic.
//
// Applications that need full control over the listener and http.Server should
// call NewConnectHandler and serve the returned handler themselves.
type ConnectServerOptions struct {
	// Services register their generated handlers on the internal mux.
	Services []ConnectService
	// HandlerOptions are applied to every service (connect.WithInterceptors,
	// connect.WithReadMaxBytes, and so on).
	HandlerOptions []connect.HandlerOption
	// Interceptors are extra application interceptors appended after the
	// config-driven authentication interceptor.
	Interceptors []connect.Interceptor
	// DisableAuthFromConfig prevents auth interceptors from being installed
	// automatically. Applications that authenticate requests themselves should
	// set this and supply their own Interceptors.
	DisableAuthFromConfig bool
	// WebHandler serves non-RPC requests, for example embedded Flutter assets.
	WebHandler http.Handler
	// SPA enables an index.html fallback for client-side routing. When set it
	// replaces WebHandler and serves assets from SPAOptions.Assets.
	SPA *SPAOptions
	// ServiceNames are the fully qualified services exposed for health checks
	// and reflection. They are required when those features are enabled.
	ServiceNames []string
	// AllowedOrigins is the CORS allow-list for browser gRPC-Web traffic. An
	// empty list falls back to grpc.server.allowedorigins. Entries must be
	// exact origins; wildcards are rejected configuration.
	AllowedOrigins []string
	// TLSEnabled serves HTTPS/HTTP2 instead of plaintext (h2c).
	TLSEnabled bool
	// TLSCertFileName and TLSKeyFileName are used when TLSEnabled is true.
	TLSCertFileName string
	TLSKeyFileName  string
}

// Upstream note: Connect (connectrpc.com/connect) replaces the archived
// github.com/improbable-eng/grpc-web wrapper for gRPC-Web translation. Before
// removing this dependency, document the replacement upstream fix and the
// migration path.
func buildConnectHandlerOptions(conf *Config, options ConnectServerOptions) ([]connect.HandlerOption, error) {
	interceptors := make([]connect.Interceptor, 0, len(options.Interceptors)+4)
	// Outermost first: recovery, request id, access logging, then auth, then
	// consumer interceptors. Auth failures are therefore logged, and panics are
	// recovered regardless of which layer panics.
	interceptors = append(interceptors, newConnectDefaultInterceptors(conf)...)

	if !options.DisableAuthFromConfig {
		authInterceptor, err := newConnectAuthInterceptor(conf)
		if err != nil {
			return nil, err
		}
		if authInterceptor != nil {
			interceptors = append(interceptors, authInterceptor)
		}
	}
	interceptors = append(interceptors, options.Interceptors...)

	handlerOptions := make([]connect.HandlerOption, 0, len(options.HandlerOptions)+len(interceptors)+1)
	handlerOptions = append(handlerOptions, options.HandlerOptions...)
	for _, interceptor := range interceptors {
		handlerOptions = append(handlerOptions, connect.WithInterceptors(interceptor))
	}
	if conf.Grpc.Server.MaxReceiveMessageBytes > 0 {
		handlerOptions = append(handlerOptions, connect.WithReadMaxBytes(conf.Grpc.Server.MaxReceiveMessageBytes))
	}

	return handlerOptions, nil
}

// NewConnectHandler builds an http.Handler serving Connect, gRPC, and gRPC-Web
// on one origin, with optional embedded web assets and CORS preflight handling.
// The handler is wrapped in h2c when TLS is disabled so native gRPC clients can
// use plaintext HTTP/2.
func NewConnectHandler(options ConnectServerOptions) (http.Handler, error) {
	conf, err := GetPlatformConfiguration()
	if err != nil {
		return nil, err
	}
	if conf == nil {
		conf = createDefaultConfiguration()
	}
	initializeMetrics(conf)

	if (conf.Grpc.Server.HealthEnabled || conf.Grpc.Server.ReflectionEnabled) && len(options.ServiceNames) == 0 {
		return nil, errors.New("grpc health/reflection require ConnectServerOptions.ServiceNames (fully qualified service names such as wdid.v1.AuthService)")
	}

	handlerOptions, err := buildConnectHandlerOptions(conf, options)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	for _, service := range options.Services {
		if service == nil {
			continue
		}
		service.Register(mux, handlerOptions...)
	}

	if conf.Grpc.Server.HealthEnabled {
		checker := grpchealth.NewStaticChecker(options.ServiceNames...)
		prefix, healthHandler := grpchealth.NewHandler(checker, handlerOptions...)
		mux.Handle(prefix, healthHandler)
	}

	if conf.Grpc.Server.ReflectionEnabled {
		reflector := grpcreflect.NewStaticReflector(options.ServiceNames...)
		prefixV1, handlerV1 := grpcreflect.NewHandlerV1(reflector, handlerOptions...)
		mux.Handle(prefixV1, handlerV1)
		prefixV1Alpha, handlerV1Alpha := grpcreflect.NewHandlerV1Alpha(reflector, handlerOptions...)
		mux.Handle(prefixV1Alpha, handlerV1Alpha)
	}

	var webHandler http.Handler
	if options.SPA != nil {
		webHandler = NewSPAHandler(options.SPA.Assets, options.SPA.Index)
	} else if options.WebHandler != nil {
		webHandler = options.WebHandler
	}

	handler := withRPCAwareFallback(mux, webHandler)

	allowedOrigins := options.AllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = conf.Grpc.Server.AllowedOrigins
	}
	handler = corsMiddleware(handler, allowedOrigins, true)

	// The server enables unencrypted HTTP/2 (h2c) via http.Server.Protocols
	// when TLS is disabled; see configureHTTPProtocols.

	return handler, nil
}

func newConnectHTTPServer(conf *Config, handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: time.Duration(conf.Grpc.Server.ReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout:       time.Duration(conf.Grpc.Server.ReadTimeoutSeconds) * time.Second,
		// WriteTimeout stays 0: unary RPCs are bounded by an interceptor
		// deadline, while server streams use StreamTimeoutSeconds (0 = no limit).
		WriteTimeout: 0,
		IdleTimeout:  time.Duration(conf.Grpc.Server.IdleTimeoutSeconds) * time.Second,
	}
}

// ServeConnect serves Connect, gRPC, and gRPC-Web on a caller-owned listener
// until the context is cancelled, then shuts down gracefully.
func ServeConnect(ctx context.Context, listener net.Listener, options ConnectServerOptions) error {
	if listener == nil {
		return errors.New("nil connect listener")
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

	handler, err := NewConnectHandler(options)
	if err != nil {
		return err
	}

	return serveHTTPServer(ctx, newConnectHTTPServer(conf, handler), listener,
		options.TLSEnabled, options.TLSCertFileName, options.TLSKeyFileName)
}

// StartConnectServerWithContext listens on the configured gRPC address and
// serves until the context is cancelled. TLS, certificate paths, and timeouts
// come from grpc.server in the platform configuration; the listener is owned by
// this function.
func StartConnectServerWithContext(ctx context.Context, options ConnectServerOptions) error {
	conf, err := GetPlatformConfiguration()
	if err != nil {
		Log.Error("Error reading platform configuration", zap.Error(err))
		return err
	}
	if conf == nil {
		conf = createDefaultConfiguration()
	}

	options.TLSEnabled = conf.Grpc.Server.TLSEnabled
	options.TLSCertFileName = conf.Grpc.Server.TLSCertFileName
	options.TLSKeyFileName = conf.Grpc.Server.TLSKeyFileName

	address := conf.Grpc.Server.ListeningAddress
	if address == "" {
		address = "127.0.0.1:9001"
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		Log.Error("Unable to listen for gRPC traffic", zap.Error(err))
		return err
	}
	defer listener.Close()

	Log.Info("Starting Connect server", zap.String("ListeningAddress", address), zap.Bool("tls", options.TLSEnabled))

	startMetricsServerIfEnabled(ctx, conf)

	return ServeConnect(ctx, listener, options)
}
