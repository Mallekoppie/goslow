package platform

import "fmt"

// AuthStrategy enumerates supported auth modes for a route.
type AuthStrategy string

const (
	AuthNone        AuthStrategy = "none"
	AuthLocalJWT    AuthStrategy = "local-jwt"
	AuthOAuth       AuthStrategy = "oauth"
	AuthBasic       AuthStrategy = "basic"
	AuthLocalOrOIDC AuthStrategy = "local-or-oidc"
)

func (a AuthStrategy) Enabled() bool {
	return a != "" && a != AuthNone
}

// routeAuthStrategies resolves the strategies for a route. An explicit Route.Auth
// wins; otherwise the legacy AuthRequired flag keeps today's AND semantics over
// every enabled server strategy.
func routeAuthStrategies(conf *Config, route Route) ([]AuthStrategy, error) {
	if route.Auth != "" {
		switch route.Auth {
		case AuthNone:
			return nil, nil
		case AuthLocalJWT, AuthOAuth, AuthBasic:
			return []AuthStrategy{route.Auth}, nil
		case AuthLocalOrOIDC:
			return []AuthStrategy{AuthLocalJWT, AuthOAuth}, nil
		default:
			return nil, fmt.Errorf("unknown auth strategy %q", route.Auth)
		}
	}

	if !route.AuthRequired {
		return nil, nil
	}

	strategies := make([]AuthStrategy, 0, 3)
	if conf.Auth.Server.Basic.Enabled {
		strategies = append(strategies, AuthBasic)
	}
	if conf.Auth.Server.OAuth.Enabled {
		strategies = append(strategies, AuthOAuth)
	}
	if conf.Auth.Server.LocalJwt.Enabled {
		strategies = append(strategies, AuthLocalJWT)
	}
	return strategies, nil
}

func authStrategyVerifier(conf *Config, strategy AuthStrategy, roles []string) (authVerifier, error) {
	switch strategy {
	case AuthBasic:
		if !conf.Auth.Server.Basic.Enabled {
			return nil, fmt.Errorf("auth strategy %q requested but basic auth is not enabled", strategy)
		}
		return basicAuthVerifier(conf.Auth.Server.Basic.AllowedUsers), nil
	case AuthOAuth:
		if !conf.Auth.Server.OAuth.Enabled {
			return nil, fmt.Errorf("auth strategy %q requested but oauth is not enabled", strategy)
		}
		return oauthVerifier(roles), nil
	case AuthLocalJWT:
		if !conf.Auth.Server.LocalJwt.Enabled {
			return nil, fmt.Errorf("auth strategy %q requested but local jwt is not enabled", strategy)
		}
		return localJWTVerifier(), nil
	default:
		return nil, fmt.Errorf("unknown auth strategy %q", strategy)
	}
}
