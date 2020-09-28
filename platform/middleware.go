package platform

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	"go.uber.org/zap"
)

const (
	ContextOAuthRoles string = "OAuthRoles"
)

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w}
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}

	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.wroteHeader = true

	return
}

func (rw *responseWriter) Status() int {
	if rw.status == 0 {
		return 200
	}

	return rw.status
}

func loggingMiddleware(next http.Handler, slaMs int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := wrapResponseWriter(w)
		next.ServeHTTP(wrapped, r)

		difference := time.Since(start).Milliseconds()

		Logger.Info("Request completed",
			zap.Int("statuscode", wrapped.Status()),
			zap.String("method", r.Method),
			zap.String("path", r.URL.EscapedPath()),
			zap.Int64("duration", difference))

		if slaMs > 0 {
			if difference > slaMs {
				Logger.Warn("SLA contract exceeded",
					zap.Int64("SLA", slaMs),
					zap.String("method", r.Method),
					zap.String("path", r.URL.EscapedPath()),
					zap.Int64("duration", difference))
			}
		}
	})
}

type customClaims struct {
	Roles []string `json:"roles,omitempty"`
}

func oAuth2Middleware(inner http.Handler, roles []string) http.Handler {

	ctx := context.Background()
	// TODO: Get from config
	provider, err := oidc.NewProvider(ctx, "http://localhost:8180/auth/realms/golang") // this is bad
	if err != nil {
		panic(err)
	}

	// TODO: Get from config
	oidcConfig := &oidc.Config{
		ClientID: "gotutorial", // this is bad
	}
	verifier := provider.Verifier(oidcConfig)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		authorized := false
		injectRolesToContext := make([]string, 0)
		// Only check the claims if a specific role is required for the path
		//if 1 == 0 {
		if len(roles) > 0 {
			authorized = false
			rawAccessToken := r.Header.Get("Authorization")

			if rawAccessToken == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			parts := strings.Split(rawAccessToken, " ")
			if len(parts) != 2 {
				Logger.Error("Auth header not build correctly")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			idToken, err := verifier.Verify(ctx, parts[1])
			if err != nil {
				Logger.Error("Token verification failed", zap.Error(err))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			claims := customClaims{}
			err = idToken.Claims(&claims)
			if err != nil {
				Logger.Error("Unable to get claims", zap.Error(err))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			for neededlIndex := range roles {
				for tokenRoleIndex := range claims.Roles {
					if roles[neededlIndex] == claims.Roles[tokenRoleIndex] {
						authorized = true
						injectRolesToContext = append(injectRolesToContext, roles[neededlIndex])
					}
				}
			}

			if authorized != true {
				Logger.Error("Required role not found",
					zap.Strings("roles", roles),
					zap.Strings("alowedRoles", claims.Roles),
					zap.String("path", r.URL.EscapedPath()),
					zap.String("method", r.Method))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// TODO: Expand this with scopes

			// r.Header.Add("X-Token-Roles", injectRoles)
		}

		if authorized {
			ctx := context.WithValue(r.Context(), ContextOAuthRoles, injectRolesToContext)
			inner.ServeHTTP(w, r.WithContext(ctx))
		}
	})
}

func allowedContentTypeMiddleware(inner http.Handler, contentTypeConfig string) http.Handler {

	contentType := strings.ToLower(contentTypeConfig)
	enabled := len(contentTypeConfig) > 0

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if enabled {
			result := r.Header.Get("Content-Type")

			if strings.ToLower(result) == contentType {
				inner.ServeHTTP(w, r)
			} else {
				Logger.Error("Content type not allowed", zap.String("content-type", result))
				w.WriteHeader(http.StatusUnsupportedMediaType)
			}
		} else {
			inner.ServeHTTP(w, r)
		}
	})
}
