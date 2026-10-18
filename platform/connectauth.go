package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
)

// connectAuthInterceptor authenticates Connect, gRPC, and gRPC-Web requests
// using the same verifiers as the HTTP middleware. Both unary and streaming
// handlers are covered, on TLS and plaintext listeners alike.
type connectAuthInterceptor struct {
	unauthenticatedPaths map[string]struct{}
	verifiers            []authVerifier
}

// newConnectAuthInterceptor builds the config-driven interceptor. It returns a
// nil interceptor when no gRPC-compatible authentication is enabled, and an
// error when the configuration cannot protect RPCs safely.
func newConnectAuthInterceptor(conf *Config) (connect.Interceptor, error) {
	if conf == nil {
		return nil, errors.New("nil platform configuration")
	}

	verifiers := make([]authVerifier, 0, 2)
	if conf.Auth.Server.LocalJwt.Enabled {
		verifiers = append(verifiers, localJWTVerifier())
	}
	if conf.Auth.Server.OAuth.Enabled {
		verifiers = append(verifiers, oauthVerifier(nil))
	}

	if len(verifiers) == 0 {
		if conf.Auth.Server.Basic.Enabled {
			return nil, fmt.Errorf("basic auth cannot protect gRPC transports; enable auth.server.localjwt or auth.server.oauth")
		}
		return nil, nil
	}

	// Basic auth cannot be applied to native gRPC or gRPC-Web metadata. When it
	// is enabled alongside a compatible strategy, the compatible one wins.
	if conf.Auth.Server.Basic.Enabled && Log != nil {
		Log.Warn("Basic auth is ignored for gRPC transports; using local jwt/oauth instead")
	}

	unauthenticatedPaths := make(map[string]struct{}, len(conf.Grpc.Server.UnAuthenticatedPaths))
	for _, path := range conf.Grpc.Server.UnAuthenticatedPaths {
		unauthenticatedPaths[path] = struct{}{}
	}

	return &connectAuthInterceptor{
		unauthenticatedPaths: unauthenticatedPaths,
		verifiers:            verifiers,
	}, nil
}

func (i *connectAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		authorizedContext, err := i.authorize(ctx, request.Spec().Procedure, request.Header())
		if err != nil {
			return nil, err
		}
		return next(authorizedContext, request)
	}
}

func (i *connectAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *connectAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) error {
		authorizedContext, err := i.authorize(ctx, connection.Spec().Procedure, connection.RequestHeader())
		if err != nil {
			return err
		}
		return next(authorizedContext, connection)
	}
}

// authorize returns a context enriched with the claims from a successful
// verifier, or a connect error mapped from the failure.
func (i *connectAuthInterceptor) authorize(ctx context.Context, procedure string, header http.Header) (context.Context, error) {
	if _, skip := i.unauthenticatedPaths[procedure]; skip {
		return ctx, nil
	}

	if _, ok := bearerToken(header.Get("Authorization")); !ok {
		return ctx, connect.NewError(connect.CodeUnauthenticated, errors.New("missing or malformed bearer token"))
	}

	request := (&http.Request{Header: header}).WithContext(ctx)

	var lastErr error
	for _, verifier := range i.verifiers {
		result, err := verifier(request)
		if err != nil {
			lastErr = err
			continue
		}
		return applyAuthResult(ctx, result), nil
	}

	if errors.Is(lastErr, errOIDCUnavailable) {
		return ctx, connect.NewError(connect.CodeUnavailable, errors.New("identity provider unavailable"))
	}
	return ctx, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
}
