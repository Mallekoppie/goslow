package platform

import (
	"net/http"
	"strconv"
)

// statusClientClosedRequest is the conventional non-standard status for a
// request whose caller went away before a response was produced.
const statusClientClosedRequest = 499

// authorizeHTTPRoute enforces one resolved access rule for an HTTP route.
func authorizeHTTPRoute(next http.Handler, authorizer *Authorizer, rule AccessRule) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rule.public {
			recordAuthDecision("http", "anonymous")
			next.ServeHTTP(w, r)
			return
		}

		authorizedContext, err := authorizer.Authorize(r.Context(), rule, credentialFromHTTPRequest(r))
		if err != nil {
			recordAuthorizationFailure("http", err)
			writeHTTPAuthorizationError(w, err)
			return
		}

		recordAuthDecision("http", "allowed")
		next.ServeHTTP(w, r.WithContext(authorizedContext))
	})
}

// credentialFromAuthorizationHeader extracts a bearer credential from an
// Authorization header. It yields the zero Credential when no bearer token is
// present, leaving any transport-specific fallback to the adapter.
func credentialFromAuthorizationHeader(header http.Header) Credential {
	if token, ok := bearerToken(header.Get("Authorization")); ok {
		return BearerCredential(token)
	}
	return Credential{}
}

// credentialFromHTTPRequest extracts the transport-neutral proof from the
// Authorization header. A request with no usable credential yields an empty
// Credential, which the authorizer reports as authentication-required.
func credentialFromHTTPRequest(r *http.Request) Credential {
	if credential := credentialFromAuthorizationHeader(r.Header); credential.kind != credentialNone {
		return credential
	}
	if username, password, ok := r.BasicAuth(); ok {
		return BasicCredential(username, password)
	}
	return Credential{}
}

// writeHTTPAuthorizationError maps the authorizer's transport-neutral outcome
// onto status codes. A verified caller lacking a role is forbidden; an
// unavailable identity provider is retryable; everything else is
// authentication-required.
func writeHTTPAuthorizationError(w http.ResponseWriter, err error) {
	switch AuthorizationOutcome(err) {
	case AuthOutcomeCancelled:
		http.Error(w, "Client Closed Request", statusClientClosedRequest)
	case AuthOutcomeForbidden:
		http.Error(w, "Forbidden", http.StatusForbidden)
	case AuthOutcomeUnavailable:
		w.Header().Set("Retry-After", strconv.Itoa(UnavailableRetryAfterSeconds))
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	default:
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}
