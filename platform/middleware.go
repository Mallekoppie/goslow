package platform

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
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

	config, err := getPlatformConfiguration()
	if err != nil {
		Logger.Fatal("unable to load platform configuration", zap.Error(err))
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, config.Auth.Server.OAuth.IdpWellKnownURL)
	if err != nil {
		Logger.Fatal("Error communication with IDP provider", zap.Error(err), zap.String("provider_url", config.Auth.Server.OAuth.IdpWellKnownURL))
	}

	oidcConfig := &oidc.Config{
		ClientID: config.Auth.Server.OAuth.ClientID,
	}
	verifier := provider.Verifier(oidcConfig)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		authorized := false
		injectRolesToContext := make([]string, 0)
		// Only check the claims if a specific role is required for the path
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

			for neededIndex := range roles {
				for tokenRoleIndex := range claims.Roles {
					if roles[neededIndex] == claims.Roles[tokenRoleIndex] {
						authorized = true
						injectRolesToContext = append(injectRolesToContext, roles[neededIndex])
					}
				}
			}

			if authorized != true {
				Logger.Error("Required role not found",
					zap.Strings("roles", roles),
					zap.Strings("allowedRoles", claims.Roles),
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

// Adds 75-80 ms to the response time...
func basicAuthMiddleware(inner http.Handler, users map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Logger.Debug("Entered Basic Auth Middleware")
		username, password, ok := r.BasicAuth()
		if ok {
			Logger.Debug("Basic Auth returned ok")
			allowedPassword := users[username]
			Logger.Debug("Allowed password", zap.String("passwordhash", allowedPassword))

			if len(allowedPassword) < 1 {
				Logger.Error("User not allowed")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			compareError := bcrypt.CompareHashAndPassword([]byte(allowedPassword), []byte(password))

			if compareError != nil {
				Logger.Error("Password incorrect", zap.Error(compareError))
				w.WriteHeader(http.StatusUnauthorized)
				return
			} else {
				inner.ServeHTTP(w, r)
			}
		} else {
			Logger.Error("Authorization header required")
			w.WriteHeader(http.StatusUnauthorized)
		}
	})
}

func AllowCorsForLocalDevelopment(inner http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// This is bad. I'll add configuration later
		// TODO: Add configuration
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Access-Control-Allow-Methods", "*")
		w.Header().Add("Access-Control-Allow-Headers", "*")

		inner.ServeHTTP(w, r)
	})
}
