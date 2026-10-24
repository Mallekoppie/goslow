package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"go.uber.org/zap"
)

type contextKey string

const (
	contextRequestID contextKey = "request.id"

	// RequestIDHeader is the request correlation header.
	RequestIDHeader = "X-Request-ID"
)

// RequestIDFromContext returns the request correlation id.
func RequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value(contextRequestID).(string); ok {
		return requestID
	}
	return ""
}

func newRequestID() string {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer[:])
}

// requestIDMiddleware preserves an incoming request id or generates one, and
// exposes it on the response and in the request context.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(RequestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}

		w.Header().Set(RequestIDHeader, requestID)
		r.Header.Set(RequestIDHeader, requestID)
		ctx := context.WithValue(r.Context(), contextRequestID, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// recoveryMiddleware converts a handler panic into a JSON 500 instead of a
// dropped connection.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				requestID := RequestIDFromContext(r.Context())
				Log.Error("Recovered from request panic",
					zap.Any("panic", recovered),
					zap.String("method", r.Method),
					zap.String("path", r.URL.EscapedPath()),
					zap.String("request_id", requestID))

				JsonMarshaller.WriteJsonResponse(w, http.StatusInternalServerError, map[string]string{
					"error":      "internal server error",
					"request_id": requestID,
				})
			}
		}()

		next.ServeHTTP(w, r)
	})
}

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
}

func (rw *responseWriter) Status() int {
	if rw.status == 0 {
		return 200
	}
	return rw.status
}

func loggingMiddleware(next http.Handler, route string, slaMs int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		finishInflight := trackInflight("http")
		defer finishInflight()

		wrapped := wrapResponseWriter(w)
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)

		observeHTTPRequest(r.Method, route, wrapped.Status(), duration)

		if Log != nil {
			Log.Info("Request completed",
				zap.Int("statuscode", wrapped.Status()),
				zap.String("method", r.Method),
				zap.String("route", route),
				zap.String("path", r.URL.EscapedPath()),
				zap.String("request_id", RequestIDFromContext(r.Context())),
				zap.Int64("duration", duration.Milliseconds()))
		}
		if slaMs > 0 && duration.Milliseconds() > slaMs && Log != nil {
			Log.Warn("SLA contract exceeded", zap.Int64("SLA", slaMs), zap.String("method", r.Method), zap.String("route", route), zap.Int64("duration", duration.Milliseconds()))
		}
	})
}

func bearerToken(headerValue string) (string, bool) {
	if strings.TrimSpace(headerValue) == "" {
		return "", false
	}
	if !strings.HasPrefix(headerValue, "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(headerValue[len("Bearer "):])
	if token == "" {
		return "", false
	}
	return token, true
}

// idTokenAudiencePolicy states whether an ID token must be issued to this
// service's configured client.
type idTokenAudiencePolicy uint8

const (
	// requireConfiguredClientID is used for route authorization: the token
	// proves this service's own caller, so its audience must include the
	// configured client ID.
	requireConfiguredClientID idTokenAudiencePolicy = iota

	// acceptAnyAudience is used only for forwarded client tokens. The token was
	// minted for another client and is forwarded as evidence about that client,
	// so requiring this service's client ID would reject every valid token.
	// Signature, expiry, and issuer are still checked.
	acceptAnyAudience
)

type idTokenClaimsVerifier struct {
	verifier *oidc.IDTokenVerifier
}

func (v idTokenClaimsVerifier) Verify(ctx context.Context, rawToken string) (map[string]interface{}, error) {
	// go-oidc checks the context while waiting for a key fetch, but a valid
	// cached key would otherwise verify after the caller has gone away.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		// A cancelled request is not an identity-provider outage.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, classifyIDTokenVerificationError(err)
	}

	claims := make(map[string]interface{})
	if err := idToken.Claims(&claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// classifyIDTokenVerificationError separates an identity-provider outage from a
// rejected credential. go-oidc reports a failed JWKS fetch as "fetching keys ..."
// and discards the wrapped transport error, so the message is the only signal
// available at this layer. A failed key fetch says nothing about the credential.
func classifyIDTokenVerificationError(err error) error {
	if strings.Contains(err.Error(), "fetching keys") {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return err
}

var (
	newOIDCClaimsVerifier = defaultOIDCClaimsVerifier
	oidcProviderLock      sync.Mutex
	oidcProviderCache     = make(map[string]*oidc.Provider)
)

// defaultOIDCClaimsVerifier builds the one ID-token verifier used by route
// authorization and by forwarded client-token validation. The two callers
// differ only in their audience policy: routes require the configured client
// ID; forwarded client tokens accept any audience.
func defaultOIDCClaimsVerifier(ctx context.Context, conf *Config, policy idTokenAudiencePolicy) (IDTokenVerifier, error) {
	if conf == nil {
		return nil, errors.New("nil platform configuration")
	}
	if conf.Auth.Server.OAuth.IdpWellKnownURL == "" {
		return nil, errors.New("idp well-known url is not configured")
	}

	provider, err := cachedOIDCProvider(ctx, conf.Auth.Server.OAuth.IdpWellKnownURL)
	if err != nil {
		return nil, err
	}

	config := &oidc.Config{ClientID: conf.Auth.Server.OAuth.ClientID}
	if policy == acceptAnyAudience {
		config.SkipClientIDCheck = true
	}
	if len(conf.Auth.Server.OAuth.AllowedAlgorithms) > 0 {
		config.SupportedSigningAlgs = normalizeAlgorithms(conf.Auth.Server.OAuth.AllowedAlgorithms)
	}

	// provider.Verifier reuses one RemoteKeySet for the cached provider, which
	// refreshes its keys when a token presents an unknown key ID.
	return idTokenClaimsVerifier{verifier: provider.Verifier(config)}, nil
}

func cachedOIDCProvider(ctx context.Context, wellKnownURL string) (*oidc.Provider, error) {
	oidcProviderLock.Lock()
	cached := oidcProviderCache[wellKnownURL]
	oidcProviderLock.Unlock()
	if cached != nil {
		return cached, nil
	}

	provider, err := oidc.NewProvider(ctx, wellKnownURL)
	if err != nil {
		return nil, err
	}

	oidcProviderLock.Lock()
	oidcProviderCache[wellKnownURL] = provider
	oidcProviderLock.Unlock()
	return provider, nil
}

func normalizeAlgorithms(algorithms []string) []string {
	normalized := make([]string, 0, len(algorithms))
	for _, algorithm := range algorithms {
		normalized = append(normalized, strings.ToUpper(strings.TrimSpace(algorithm)))
	}
	return normalized
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
				if Log != nil {
					Log.Error("Content type not allowed", zap.String("content-type", result))
				}
				w.WriteHeader(http.StatusUnsupportedMediaType)
			}
		} else {
			inner.ServeHTTP(w, r)
		}
	})
}
