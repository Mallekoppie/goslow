package platform

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
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
	// AllowedOrigins is the CORS allow-list for browser gRPC-Web traffic. An
	// empty list falls back to grpc.server.allowedorigins and then to
	// http.server.AllowCorsForLocalDevelopment ("*").
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
	interceptors := make([]connect.Interceptor, 0, len(options.Interceptors)+1)

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

	var handler http.Handler = mux
	if options.WebHandler != nil {
		handler = withWebFallback(mux, options.WebHandler)
	}

	allowedOrigins := options.AllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = conf.Grpc.Server.AllowedOrigins
	}
	if len(allowedOrigins) == 0 && conf.HTTP.Server.AllowCorsForLocalDevelopment {
		allowedOrigins = []string{"*"}
	}
	handler = corsMiddleware(handler, allowedOrigins)

	if !options.TLSEnabled {
		handler = h2c.NewHandler(handler, &http2.Server{})
	}

	return handler, nil
}

// withWebFallback serves RPC routes from the mux and everything else from web.
func withWebFallback(rpc *http.ServeMux, web http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := rpc.Handler(r); pattern != "" {
			rpc.ServeHTTP(w, r)
			return
		}
		web.ServeHTTP(w, r)
	})
}

var (
	connectAllowedHeaders = []string{
		"Content-Type",
		"Authorization",
		"X-Grpc-Web",
		"X-User-Agent",
		"Grpc-Timeout",
		"Grpc-Encoding",
		"Grpc-Accept-Encoding",
		"Connect-Protocol-Version",
		"Connect-Timeout-Ms",
		"Connect-Accept-Encoding",
		"Connect-Content-Encoding",
	}
	connectExposedHeaders = []string{
		"Grpc-Status",
		"Grpc-Message",
		"Grpc-Status-Details-Bin",
		"Grpc-Encoding",
		"Connect-Protocol-Version",
		"Connect-Content-Encoding",
	}
)

func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	if len(allowedOrigins) == 0 {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowValue, allowed := allowedOriginValue(origin, allowedOrigins)
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", allowValue)
			w.Header().Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(connectAllowedHeaders, ", "))
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(connectExposedHeaders, ", "))
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func allowedOriginValue(origin string, allowedOrigins []string) (string, bool) {
	if origin == "" {
		return "", false
	}
	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			return "*", true
		}
		if strings.EqualFold(strings.TrimSpace(allowed), origin) {
			return origin, true
		}
	}
	return "", false
}

func newConnectHTTPServer(conf *Config, handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: time.Duration(conf.Grpc.Server.ReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout:       time.Duration(conf.Grpc.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(conf.Grpc.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(conf.Grpc.Server.IdleTimeoutSeconds) * time.Second,
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

	return ServeConnect(ctx, listener, options)
}
