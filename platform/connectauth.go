package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
)

// connectAuthInterceptor authenticates Connect, gRPC, and gRPC-Web requests
// using an explicit strategy and enforces per-procedure role requirements.
type connectAuthInterceptor struct {
	public         publicPathMatcher
	procedureRoles map[string][]string
	verifiers      []authVerifier
}

// newConnectAuthInterceptor builds the config-driven interceptor. An empty
// strategy is only allowed when no auth mechanism is enabled; otherwise startup
// fails closed. AuthNone is the explicit public configuration.
func newConnectAuthInterceptor(conf *Config) (connect.Interceptor, error) {
	if conf == nil {
		return nil, errors.New("nil platform configuration")
	}

	public, err := publicPaths(conf)
	if err != nil {
		return nil, err
	}

	strategy := conf.Grpc.Server.Auth
	if strategy == "" {
		if conf.Auth.Server.LocalJwt.Enabled || conf.Auth.Server.OAuth.Enabled || conf.Auth.Server.Basic.Enabled {
			return nil, ErrConnectAuthStrategyRequired
		}
		strategy = AuthNone
	}

	verifiers := make([]authVerifier, 0, 2)
	switch strategy {
	case AuthNone:
		return nil, nil
	case AuthLocalJWT:
		if !conf.Auth.Server.LocalJwt.Enabled {
			return nil, fmt.Errorf("grpc auth strategy %q requires auth.server.localjwt.enabled", strategy)
		}
		verifiers = append(verifiers, localJWTVerifier())
	case AuthOAuth:
		if !conf.Auth.Server.OAuth.Enabled {
			return nil, fmt.Errorf("grpc auth strategy %q requires auth.server.oauth.enabled", strategy)
		}
		verifiers = append(verifiers, oauthVerifier(nil))
	case AuthLocalOrOIDC:
		if !conf.Auth.Server.LocalJwt.Enabled || !conf.Auth.Server.OAuth.Enabled {
			return nil, fmt.Errorf("grpc auth strategy %q requires both local jwt and oauth to be enabled", strategy)
		}
		verifiers = append(verifiers, localJWTVerifier(), oauthVerifier(nil))
	case AuthBasic:
		return nil, fmt.Errorf("basic auth cannot protect gRPC transports; use local-jwt, oauth, or local-or-oidc")
	default:
		return nil, fmt.Errorf("unknown grpc auth strategy %q", strategy)
	}

	procedureRoles := conf.Grpc.Server.ProcedureRoles
	if procedureRoles == nil {
		procedureRoles = map[string][]string{}
	}

	return &connectAuthInterceptor{
		public:         public,
		procedureRoles: procedureRoles,
		verifiers:      verifiers,
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

// authorize returns a context enriched with the verified identity, or a connect
// error mapped from the failure.
func (i *connectAuthInterceptor) authorize(ctx context.Context, procedure string, header http.Header) (context.Context, error) {
	if i.public.matches("", procedure) {
		recordAuthDecision(rpcTransport(header), "anonymous")
		return ctx, nil
	}

	if _, ok := bearerToken(header.Get("Authorization")); !ok {
		recordAuthDecision(rpcTransport(header), "denied")
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
		if err := i.checkRoles(procedure, result); err != nil {
			recordAuthDecision(rpcTransport(header), "denied")
			return ctx, err
		}
		authorizedContext := applyAuthResult(ctx, result)
		recordAuthDecision(rpcTransport(header), "allowed")
		return authorizedContext, nil
	}

	recordAuthDecision(rpcTransport(header), "denied")
	if errors.Is(lastErr, errOIDCUnavailable) {
		return ctx, connect.NewError(connect.CodeUnavailable, errors.New("identity provider unavailable"))
	}
	return ctx, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
}

func (i *connectAuthInterceptor) checkRoles(procedure string, result authResult) error {
	required := i.procedureRoles[procedure]
	if len(required) == 0 {
		return nil
	}

	granted := result.Roles
	if len(granted) == 0 {
		granted = rolesFromClaims(result.LocalClaims)
	}

	for _, needed := range required {
		for _, held := range granted {
			if needed == held {
				return nil
			}
		}
	}

	return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("missing required role for %s", procedure))
}

// rolesFromClaims extracts roles from local JWT claims.
func rolesFromClaims(claims map[string]interface{}) []string {
	if len(claims) == 0 {
		return nil
	}

	switch typed := claims["roles"].(type) {
	case []string:
		return typed
	case []interface{}:
		roles := make([]string, 0, len(typed))
		for _, item := range typed {
			if role, ok := item.(string); ok {
				roles = append(roles, role)
			}
		}
		return roles
	}

	if role, ok := claims["role"].(string); ok {
		return []string{role}
	}
	return nil
}
