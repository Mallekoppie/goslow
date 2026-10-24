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
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const (
	contextOAuthRoles             contextKey = "oauth.roles"
	contextOAuthClientToken       contextKey = "oauth.client.token"
	contextOAuthPreferredUsername contextKey = "oauth.preferred.username"
	contextOAuthIssuer            contextKey = "oauth.issuer"
	contextLocalJwtClaims         contextKey = "localjwt.claims"
	contextRequestID              contextKey = "request.id"

	// RequestIDHeader is the request correlation header.
	RequestIDHeader = "X-Request-ID"
)

// RolesFromContext returns the roles granted by the auth verifier.
func RolesFromContext(ctx context.Context) []string {
	if roles, ok := ctx.Value(contextOAuthRoles).([]string); ok {
		return roles
	}
	return nil
}

// ClientTokenFromContext returns the raw OAuth client token. Never log it.
func ClientTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value(contextOAuthClientToken).(string); ok {
		return token
	}
	return ""
}

// UsernameFromContext returns the authenticated username when supplied.
func UsernameFromContext(ctx context.Context) string {
	if username, ok := ctx.Value(contextOAuthPreferredUsername).(string); ok {
		return username
	}
	return ""
}

// IssuerFromContext returns the OAuth issuer.
func IssuerFromContext(ctx context.Context) string {
	if issuer, ok := ctx.Value(contextOAuthIssuer).(string); ok {
		return issuer
	}
	return ""
}

// ClaimsFromContext returns local JWT claims.
func ClaimsFromContext(ctx context.Context) map[string]interface{} {
	if claims, ok := ctx.Value(contextLocalJwtClaims).(map[string]interface{}); ok {
		return claims
	}
	return nil
}

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

var (
	ErrAuthRequired    = errors.New("authentication required")
	errOIDCUnavailable = errors.New("oidc provider unavailable")
)

// authResult carries everything an auth strategy extracted from the request.
type authResult struct {
	Roles       []string
	Token       string
	Issuer      string
	Username    string
	LocalClaims map[string]interface{}
}

// authVerifier authenticates a request or returns an error.
type authVerifier func(*http.Request) (authResult, error)

func mergeAuthResults(results ...authResult) authResult {
	merged := authResult{}
	for _, result := range results {
		if len(result.Roles) > 0 {
			merged.Roles = append(merged.Roles, result.Roles...)
		}
		if result.Token != "" {
			merged.Token = result.Token
		}
		if result.Issuer != "" {
			merged.Issuer = result.Issuer
		}
		if result.Username != "" {
			merged.Username = result.Username
		}
		if len(result.LocalClaims) > 0 {
			merged.LocalClaims = result.LocalClaims
		}
	}
	return merged
}

func applyAuthResult(ctx context.Context, result authResult) context.Context {
	if len(result.Roles) > 0 {
		ctx = context.WithValue(ctx, contextOAuthRoles, result.Roles)
	}
	if result.Token != "" {
		ctx = context.WithValue(ctx, contextOAuthClientToken, result.Token)
	}
	if result.Issuer != "" {
		ctx = context.WithValue(ctx, contextOAuthIssuer, result.Issuer)
	}
	if result.Username != "" {
		ctx = context.WithValue(ctx, contextOAuthPreferredUsername, result.Username)
	}
	if len(result.LocalClaims) > 0 {
		ctx = context.WithValue(ctx, contextLocalJwtClaims, result.LocalClaims)
	}
	return ctx
}

func writeAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, errOIDCUnavailable) {
		w.Header().Set("Retry-After", "10")
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// anyAuthMiddleware succeeds when at least one verifier accepts the request.
func anyAuthMiddleware(inner http.Handler, verifiers ...authVerifier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(verifiers) == 0 {
			inner.ServeHTTP(w, r)
			return
		}

		results := make([]authResult, 0, len(verifiers))
		var lastErr error
		for _, verifier := range verifiers {
			result, err := verifier(r)
			if err != nil {
				lastErr = err
				continue
			}
			results = append(results, result)
		}

		if len(results) == 0 {
			recordAuthDecision("http", "denied")
			writeAuthError(w, lastErr)
			return
		}

		ctx := applyAuthResult(r.Context(), mergeAuthResults(results...))
		inner.ServeHTTP(w, r.WithContext(ctx))
	})
}

// allAuthMiddleware requires every verifier to accept the request.
func allAuthMiddleware(inner http.Handler, verifiers ...authVerifier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results := make([]authResult, 0, len(verifiers))
		for _, verifier := range verifiers {
			result, err := verifier(r)
			if err != nil {
				recordAuthDecision("http", "denied")
				writeAuthError(w, err)
				return
			}
			results = append(results, result)
		}

		ctx := applyAuthResult(r.Context(), mergeAuthResults(results...))
		inner.ServeHTTP(w, r.WithContext(ctx))
	})
}

func basicAuthVerifier(users map[string]string) authVerifier {
	return func(r *http.Request) (authResult, error) {
		username, password, ok := r.BasicAuth()
		if !ok {
			return authResult{}, ErrAuthRequired
		}

		allowedPassword, found := users[username]
		if !found || len(allowedPassword) < 1 {
			return authResult{}, ErrAuthRequired
		}

		if err := bcrypt.CompareHashAndPassword([]byte(allowedPassword), []byte(password)); err != nil {
			return authResult{}, ErrAuthRequired
		}

		return authResult{Username: username}, nil
	}
}

func localJWTVerifier() authVerifier {
	return func(r *http.Request) (authResult, error) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			return authResult{}, ErrAuthRequired
		}

		claims, err := LocalJwt.ValidateLocalJwtToken(token)
		if err != nil {
			return authResult{}, err
		}

		return authResult{Token: token, LocalClaims: claims}, nil
	}
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

// extractRoles reads roles from the configured claim. The literal claim path is
// tried first (Zitadel-style URNs), then as a dotted path. Values may be a list
// of strings or an object whose keys are the roles.
func extractRoles(claims map[string]interface{}, claimPath string) []string {
	roles, _ := rolesFromClaim(claims, claimPath)
	return roles
}

func oauthVerifier(roles []string) authVerifier {
	return func(r *http.Request) (authResult, error) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			return authResult{}, ErrAuthRequired
		}

		conf, err := GetPlatformConfiguration()
		if err != nil || conf == nil {
			return authResult{}, errOIDCUnavailable
		}

		verifierFactory := newOIDCClaimsVerifier
		verifier, err := verifierFactory(r.Context(), conf, requireConfiguredClientID)
		if err != nil {
			if Log != nil {
				Log.Error("Error communicating with IDP provider", zap.Error(err), zap.String("provider_url", conf.Auth.Server.OAuth.IdpWellKnownURL))
			}
			return authResult{}, fmt.Errorf("%w: %v", errOIDCUnavailable, err)
		}

		claims, err := verifier.Verify(r.Context(), token)
		if err != nil {
			if Log != nil {
				Log.Error("Token verification failed", zap.Error(err))
			}
			return authResult{}, ErrAuthRequired
		}

		grantedRoles := extractRoles(claims, conf.Auth.Server.OAuth.RolesClaim)
		if len(roles) > 0 {
			matched := make([]string, 0, len(roles))
			for _, required := range roles {
				for _, granted := range grantedRoles {
					if required == granted {
						matched = append(matched, required)
						break
					}
				}
			}
			if len(matched) == 0 {
				if Log != nil {
					Log.Error("Required role not found", zap.Strings("roles", roles), zap.Strings("allowedRoles", grantedRoles), zap.String("path", r.URL.EscapedPath()), zap.String("method", r.Method))
				}
				return authResult{}, ErrAuthRequired
			}
			grantedRoles = matched
		}

		result := authResult{Roles: grantedRoles, Token: token}
		if issuer, ok := claims["iss"].(string); ok {
			result.Issuer = issuer
		}
		if username, ok := claims["preferred_username"].(string); ok {
			result.Username = username
		}

		return result, nil
	}
}

func oAuth2Middleware(inner http.Handler, roles []string) http.Handler {
	return allAuthMiddleware(inner, oauthVerifier(roles))
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

func basicAuthMiddleware(inner http.Handler, users map[string]string) http.Handler {
	return allAuthMiddleware(inner, basicAuthVerifier(users))
}

func localJwtAuthMiddleware(next http.Handler) http.Handler {
	return allAuthMiddleware(next, localJWTVerifier())
}
