package platform

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/zap"
)

// newConnectDefaultInterceptors returns the platform interceptors in execution
// order (outermost first): request id, panic recovery, access logging.
// Request id wraps recovery so recovered panics can still be correlated.
// Authentication is inserted after these so failures are logged and panics are
// recovered, while auth still runs before consumer interceptors and handlers.
func newConnectDefaultInterceptors(conf *Config) []connect.Interceptor {
	slaMs := int64(0)
	if conf != nil {
		slaMs = conf.Grpc.Server.SlaMs
	}
	return []connect.Interceptor{
		connectRequestIDInterceptor{},
		newConnectTimeoutInterceptor(conf),
		connectRecoveryInterceptor{},
		connectLoggingInterceptor{slaMs: slaMs},
	}
}

// connectTimeoutInterceptor bounds unary RPCs with the configured write timeout
// and streams with the dedicated stream timeout. A zero stream timeout means
// unlimited, which is why the HTTP server write timeout is disabled.
type connectTimeoutInterceptor struct {
	unaryTimeout  time.Duration
	streamTimeout time.Duration
}

func newConnectTimeoutInterceptor(conf *Config) connect.Interceptor {
	interceptor := connectTimeoutInterceptor{}
	if conf != nil {
		interceptor.unaryTimeout = time.Duration(conf.Grpc.Server.WriteTimeoutSeconds) * time.Second
		interceptor.streamTimeout = time.Duration(conf.Grpc.Server.StreamTimeoutSeconds) * time.Second
	}
	return interceptor
}

func (i connectTimeoutInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		if i.unaryTimeout > 0 {
			timeoutContext, cancel := context.WithTimeout(ctx, i.unaryTimeout)
			defer cancel()
			ctx = timeoutContext
		}
		return next(ctx, request)
	}
}

func (i connectTimeoutInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i connectTimeoutInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) error {
		if i.streamTimeout > 0 {
			timeoutContext, cancel := context.WithTimeout(ctx, i.streamTimeout)
			defer cancel()
			ctx = timeoutContext
		}
		return next(ctx, connection)
	}
}

type connectRecoveryInterceptor struct{}

func (connectRecoveryInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (response connect.AnyResponse, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				Log.Error("Recovered from RPC panic",
					zap.Any("panic", recovered),
					zap.String("procedure", request.Spec().Procedure),
					zap.String("request_id", RequestIDFromContext(ctx)))
				err = connect.NewError(connect.CodeInternal, errors.New("internal server error"))
			}
		}()
		return next(ctx, request)
	}
}

func (connectRecoveryInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (connectRecoveryInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				Log.Error("Recovered from streaming RPC panic",
					zap.Any("panic", recovered),
					zap.String("procedure", connection.Spec().Procedure),
					zap.String("request_id", RequestIDFromContext(ctx)))
				err = connect.NewError(connect.CodeInternal, errors.New("internal server error"))
			}
		}()
		return next(ctx, connection)
	}
}

type connectRequestIDInterceptor struct{}

func (connectRequestIDInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		requestID := strings.TrimSpace(request.Header().Get(RequestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}
		ctx = context.WithValue(ctx, contextRequestID, requestID)

		response, err := next(ctx, request)
		if err != nil {
			return nil, withConnectRequestID(err, requestID)
		}
		response.Header().Set(RequestIDHeader, requestID)
		return response, nil
	}
}

func (connectRequestIDInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (connectRequestIDInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) error {
		requestID := strings.TrimSpace(connection.RequestHeader().Get(RequestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}
		ctx = context.WithValue(ctx, contextRequestID, requestID)
		connection.ResponseHeader().Set(RequestIDHeader, requestID)

		if err := next(ctx, connection); err != nil {
			return withConnectRequestID(err, requestID)
		}
		return nil
	}
}

// withConnectRequestID attaches the request id to any RPC failure so callers can
// correlate errors even when no response headers are written.
func withConnectRequestID(err error, requestID string) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		wrapped := connect.NewError(connectErr.Code(), connectErr)
		wrapped.Meta().Set(RequestIDHeader, requestID)
		return wrapped
	}

	wrapped := connect.NewError(connect.CodeUnknown, err)
	wrapped.Meta().Set(RequestIDHeader, requestID)
	return wrapped
}

type connectLoggingInterceptor struct {
	slaMs int64
}

func (i connectLoggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		start := time.Now()
		finishInflight := trackInflight("rpc")
		defer finishInflight()

		response, err := next(ctx, request)
		duration := time.Since(start)
		transport := rpcTransport(request.Header())
		observeRPCRequest(transport, request.Spec().Procedure, connect.CodeOf(err).String(), duration)
		i.log(ctx, request.Spec().Procedure, transport, connect.CodeOf(err), duration)
		return response, err
	}
}

func (connectLoggingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i connectLoggingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) error {
		start := time.Now()
		finishInflight := trackInflight("rpc")
		defer finishInflight()

		err := next(ctx, connection)
		duration := time.Since(start)
		transport := rpcTransport(connection.RequestHeader())
		observeRPCRequest(transport, connection.Spec().Procedure, connect.CodeOf(err).String(), duration)
		i.log(ctx, connection.Spec().Procedure, transport, connect.CodeOf(err), duration)
		return err
	}
}

func (i connectLoggingInterceptor) log(ctx context.Context, procedure, transport string, code connect.Code, duration time.Duration) {
	if Log == nil {
		return
	}

	Log.Info("RPC completed",
		zap.String("procedure", procedure),
		zap.String("transport", transport),
		zap.String("code", code.String()),
		zap.Int64("duration", duration.Milliseconds()),
		zap.String("request_id", RequestIDFromContext(ctx)))

	if i.slaMs > 0 && duration.Milliseconds() > i.slaMs {
		Log.Warn("RPC SLA exceeded",
			zap.String("procedure", procedure),
			zap.Int64("sla", i.slaMs),
			zap.Int64("duration", duration.Milliseconds()),
			zap.String("request_id", RequestIDFromContext(ctx)))
	}
}

// rpcTransport classifies the wire protocol from the request content type.
func rpcTransport(header http.Header) string {
	contentType := strings.ToLower(header.Get("Content-Type"))
	if index := strings.IndexByte(contentType, ';'); index >= 0 {
		contentType = contentType[:index]
	}
	contentType = strings.TrimSpace(contentType)

	switch {
	case strings.HasPrefix(contentType, "application/grpc-web"):
		return "grpc-web"
	case strings.HasPrefix(contentType, "application/grpc"):
		return "grpc"
	default:
		return "connect"
	}
}
