package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
)

// connectAuthInterceptor authorizes Connect, gRPC, and gRPC-Web procedures
// through the identity authorizer. Procedure rules are resolved once, when the
// interceptor is built, against the authorizer's settings snapshot.
type connectAuthInterceptor struct {
	authorizer     *Authorizer
	public         publicPathMatcher
	defaultRule    AccessRule
	procedureRules map[string]AccessRule
}

// newConnectAuthInterceptor builds the config-driven interceptor with its own
// authorizer. An empty strategy is only allowed when no auth mechanism is
// enabled; otherwise startup fails closed. AuthNone is the explicit public
// configuration.
func newConnectAuthInterceptor(conf *Config) (connect.Interceptor, error) {
	return newConnectAuthInterceptorWithAuthorizer(conf, nil)
}

// newConnectAuthInterceptorWithAuthorizer builds the interceptor around a
// supplied authorizer, or one constructed from conf when none is supplied. The
// combined application passes the same authorizer to the HTTP and RPC adapters
// so both share one settings snapshot and provider cache.
func newConnectAuthInterceptorWithAuthorizer(conf *Config, authorizer *Authorizer) (connect.Interceptor, error) {
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

	if err := validateProcedureRules(conf, public, strategy); err != nil {
		return nil, err
	}

	switch strategy {
	case AuthNone:
		return nil, nil
	case AuthBasic:
		return nil, fmt.Errorf("basic auth cannot protect gRPC transports; use local-jwt, oauth, or local-or-oidc")
	}

	if authorizer == nil {
		authorizer, err = newAuthorizerFromConfig(conf)
		if err != nil {
			return nil, err
		}
	}

	defaultRule, err := authorizer.ResolveAccessRule(strategy, nil)
	if err != nil {
		return nil, err
	}

	procedureRules := make(map[string]AccessRule, len(conf.Grpc.Server.ProcedureRoles))
	for procedure, roles := range conf.Grpc.Server.ProcedureRoles {
		rule, err := authorizer.ResolveAccessRule(strategy, roles)
		if err != nil {
			return nil, fmt.Errorf("procedure %s: %w", procedure, err)
		}
		procedureRules[procedure] = rule
	}

	return &connectAuthInterceptor{
		authorizer:     authorizer,
		public:         public,
		defaultRule:    defaultRule,
		procedureRules: procedureRules,
	}, nil
}

// validateProcedureRules fails closed when required roles cannot be enforced:
// roles declared on an AuthNone rule, or on a procedure the unauthenticated
// exclusion list makes public.
func validateProcedureRules(conf *Config, public publicPathMatcher, strategy AuthStrategy) error {
	for procedure, roles := range conf.Grpc.Server.ProcedureRoles {
		if len(roles) == 0 {
			continue
		}
		if public.matches("", procedure) {
			return fmt.Errorf("procedure %s declares required roles but is also in the unauthenticated exclusion list", procedure)
		}
		if strategy == AuthNone {
			return fmt.Errorf("procedure %s: %w: strategy %q does not support required roles", procedure, ErrInvalidAccessRule, AuthNone)
		}
	}
	return nil
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
// error mapped from the authorizer's failure category.
func (i *connectAuthInterceptor) authorize(ctx context.Context, procedure string, header http.Header) (context.Context, error) {
	if i.public.matches("", procedure) {
		recordAuthDecision(rpcTransport(header), "anonymous")
		return ctx, nil
	}

	rule := i.defaultRule
	if procedureRule, ok := i.procedureRules[procedure]; ok {
		rule = procedureRule
	}

	authorizedContext, err := i.authorizer.Authorize(ctx, rule, credentialFromHeader(header))
	if err != nil {
		recordAuthDecision(rpcTransport(header), "denied")
		return ctx, connectAuthorizationError(err)
	}

	recordAuthDecision(rpcTransport(header), "allowed")
	return authorizedContext, nil
}

// credentialFromHeader extracts the transport-neutral proof from RPC headers.
// RPC transports carry bearer tokens only; basic credentials are rejected when
// the interceptor is constructed.
func credentialFromHeader(header http.Header) Credential {
	if token, ok := bearerToken(header.Get("Authorization")); ok {
		return BearerCredential(token)
	}
	return Credential{}
}

// connectAuthorizationError maps the authorizer's failure categories onto RPC
// codes. A verified caller lacking a role is forbidden; an unavailable identity
// provider is retryable; everything else requires authentication.
func connectAuthorizationError(err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	case errors.Is(err, ErrUnavailable):
		return connect.NewError(connect.CodeUnavailable, errors.New("identity provider unavailable"))
	default:
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
}
