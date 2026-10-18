package platform

import (
	"net/http"
	"net/url"
	"strings"
)

// originMatcher applies an exact-origin allow-list. Wildcards are rejected by
// configuration validation so CORS never fails open.
type originMatcher struct {
	origins []string
}

func newOriginMatcher(origins []string) originMatcher {
	normalized := make([]string, 0, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		normalized = append(normalized, strings.ToLower(origin))
	}
	return originMatcher{origins: normalized}
}

// match returns the value for Access-Control-Allow-Origin and whether the
// origin is allowed.
func (m originMatcher) match(origin string) (string, bool) {
	if origin == "" {
		return "", false
	}
	for _, allowed := range m.origins {
		if strings.EqualFold(allowed, origin) {
			return origin, true
		}
	}
	return "", false
}

func (m originMatcher) empty() bool {
	return len(m.origins) == 0
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
	httpAllowedHeaders = []string{
		"Content-Type",
		"Authorization",
		"X-Request-ID",
	}
	httpExposedHeaders = []string{
		"X-Request-ID",
	}
)

// corsMiddleware answers preflight requests and adds exact-origin CORS headers.
// The same implementation is used for HTTP routes and the Connect handler.
func corsMiddleware(next http.Handler, origins []string, rpc bool) http.Handler {
	matcher := newOriginMatcher(origins)
	if matcher.empty() {
		return next
	}

	allowedHeaders := httpAllowedHeaders
	exposedHeaders := httpExposedHeaders
	if rpc {
		allowedHeaders = connectAllowedHeaders
		exposedHeaders = connectExposedHeaders
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowValue, allowed := matcher.match(origin); allowed {
			w.Header().Set("Access-Control-Allow-Origin", allowValue)
			w.Header().Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(exposedHeaders, ", "))
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// validateOrigins rejects wildcard and malformed CORS origins.
func validateOrigins(name string, origins []string) error {
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if origin == "*" {
			return &configError{message: name + " must not contain wildcard origins"}
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return &configError{message: name + " contains an invalid origin: " + origin}
		}
	}
	return nil
}

type configError struct {
	message string
}

func (e *configError) Error() string {
	return e.message
}
