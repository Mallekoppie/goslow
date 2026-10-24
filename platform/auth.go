package platform

import (
	"errors"
	"strings"
)

// AuthStrategy enumerates supported auth modes for a route.
type AuthStrategy string

const (
	AuthNone        AuthStrategy = "none"
	AuthLocalJWT    AuthStrategy = "local-jwt"
	AuthOAuth       AuthStrategy = "oauth"
	AuthBasic       AuthStrategy = "basic"
	AuthLocalOrOIDC AuthStrategy = "local-or-oidc"
)

// ErrRouteAuthStrategyRequired is returned when a route omits Auth. Public
// routes must declare AuthNone explicitly so authentication never fails open.
var ErrRouteAuthStrategyRequired = errors.New("route auth strategy is required; use AuthNone for public routes")

// ErrConnectAuthStrategyRequired is returned when Connect auth is enabled but no
// explicit strategy was configured.
var ErrConnectAuthStrategyRequired = errors.New("grpc.server.auth is required when local jwt or oauth is enabled")

// publicPathMatcher implements the unauthenticated exclusion list. Entries are
// exact paths (/metrics, /pkg.Service/Method) or method-qualified HTTP paths
// (POST /auth/login).
type publicPathMatcher struct {
	anyMethod map[string]struct{}
	byMethod  map[string]map[string]struct{}
}

func newPublicPathMatcher(entries []string) publicPathMatcher {
	matcher := publicPathMatcher{
		anyMethod: make(map[string]struct{}),
		byMethod:  make(map[string]map[string]struct{}),
	}

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		fields := strings.Fields(entry)
		if len(fields) == 2 {
			method := strings.ToUpper(fields[0])
			pathValue := fields[1]
			if matcher.byMethod[method] == nil {
				matcher.byMethod[method] = make(map[string]struct{})
			}
			matcher.byMethod[method][pathValue] = struct{}{}
			continue
		}

		matcher.anyMethod[entry] = struct{}{}
	}

	return matcher
}

// matches reports whether method+path is excluded from authentication. An empty
// method matches only unqualified entries, which is how RPC procedures are
// checked.
func (m publicPathMatcher) matches(method, pathValue string) bool {
	if _, ok := m.anyMethod[pathValue]; ok {
		return true
	}
	if method == "" {
		return false
	}
	if paths, ok := m.byMethod[strings.ToUpper(method)]; ok {
		_, excluded := paths[pathValue]
		return excluded
	}
	return false
}

// publicPaths resolves the unified exclusion list.
func publicPaths(conf *Config) (publicPathMatcher, error) {
	if conf == nil {
		return newPublicPathMatcher(nil), nil
	}
	return newPublicPathMatcher(conf.Auth.Server.UnauthenticatedPaths), nil
}

func (a AuthStrategy) Enabled() bool {
	return a != "" && a != AuthNone
}
