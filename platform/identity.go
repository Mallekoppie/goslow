package platform

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	jwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrInvalidAccessRule reports an access rule that cannot be constructed
	// from the authorizer's settings.
	ErrInvalidAccessRule = errors.New("invalid access rule")
	// ErrAuthenticationRequired reports a missing or rejected credential.
	ErrAuthenticationRequired = errors.New("authentication required")
	// ErrForbidden reports a verified caller that lacks a required role.
	ErrForbidden = errors.New("forbidden")
	// ErrUnavailable reports that the identity provider could not be reached.
	ErrUnavailable = errors.New("identity provider unavailable")
	// ErrCredentialNotApplicable lets a verifier decline a credential meant for
	// another strategy. It is never returned by Authorize.
	ErrCredentialNotApplicable = errors.New("credential not applicable")
)

// Credential is the transport-neutral proof a caller presents. Adapters extract
// it from headers; the authorizer never sees a request.
type Credential struct {
	bearerToken string
	username    string
	password    string
	kind        credentialKind
}

type credentialKind uint8

const (
	credentialNone credentialKind = iota
	credentialBearer
	credentialBasic
)

// BearerCredential wraps a raw bearer token.
func BearerCredential(token string) Credential {
	return Credential{bearerToken: token, kind: credentialBearer}
}

// BasicCredential wraps a basic-auth username and password.
func BasicCredential(username, password string) Credential {
	return Credential{username: username, password: password, kind: credentialBasic}
}

// Identity is the caller a request has been proven to be.
type Identity struct {
	Username string
	Issuer   string
	Roles    []string
	Claims   map[string]interface{}
	// ClientToken retains the raw bearer token for forwarding. Never log it.
	ClientToken string
}

type identityContextKey struct{}

// IdentityFromContext returns the identity stored by the authorizer. Public
// requests carry no identity, so callers must check the boolean.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	return identity, ok
}

// AccessRule is the resolved policy for one surface.
type AccessRule struct {
	strategy      AuthStrategy
	requiredRoles []string
	public        bool
}

// AuthSettings is the immutable snapshot the authorizer reads on every request.
type AuthSettings struct {
	LocalJWT LocalJWTSettings
	OAuth    OAuthSettings
	Basic    BasicSettings
}

// LocalJWTSettings configures local JWT verification.
type LocalJWTSettings struct {
	Enabled       bool
	SigningKey    string
	SigningMethod string
	RolesClaim    string
}

// OAuthSettings configures ID token verification. The verifier itself is
// injected through WithIDTokenVerifierFactory.
type OAuthSettings struct {
	Enabled    bool
	RolesClaim string
}

// BasicSettings configures basic credential verification.
type BasicSettings struct {
	Enabled      bool
	AllowedUsers map[string]string
}

// AuthSettingsFromConfig copies the auth-related parts of a configuration. The
// result can be handed to NewAuthorizer without retaining the config.
func AuthSettingsFromConfig(conf *Config) AuthSettings {
	if conf == nil {
		return AuthSettings{}
	}

	return AuthSettings{
		LocalJWT: LocalJWTSettings{
			Enabled:       conf.Auth.Server.LocalJwt.Enabled,
			SigningKey:    conf.Auth.Server.LocalJwt.JwtSigningKey,
			SigningMethod: conf.Auth.Server.LocalJwt.JwtSigningMethod,
			RolesClaim:    conf.Auth.Server.LocalJwt.RolesClaim,
		},
		OAuth: OAuthSettings{
			Enabled:    conf.Auth.Server.OAuth.Enabled,
			RolesClaim: conf.Auth.Server.OAuth.RolesClaim,
		},
		Basic: BasicSettings{
			Enabled:      conf.Auth.Server.Basic.Enabled,
			AllowedUsers: maps.Clone(conf.Auth.Server.Basic.AllowedUsers),
		},
	}
}

// IDTokenVerifier verifies an ID token and returns its claims.
//
// Implementations report two outcomes through errors:
//   - ErrCredentialNotApplicable when the credential is meant for another
//     strategy, so it is not counted as a definite rejection.
//   - ErrUnavailable when the identity provider cannot be reached, so the
//     authorizer can report an outage rather than a rejected credential.
//
// Any other error is a definite rejection.
type IDTokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (map[string]interface{}, error)
}

// IDTokenVerifierFactory builds the verifier for one authorization attempt. A
// factory error is reported as ErrUnavailable.
type IDTokenVerifierFactory func(ctx context.Context) (IDTokenVerifier, error)

// AuthorizerOption customizes an Authorizer at construction.
type AuthorizerOption func(*Authorizer)

// WithIDTokenVerifierFactory injects the ID token verifier factory.
func WithIDTokenVerifierFactory(factory IDTokenVerifierFactory) AuthorizerOption {
	return func(a *Authorizer) {
		a.verifierFactory = factory
	}
}

// Authorizer resolves access rules and authorizes credentials.
type Authorizer struct {
	settings        AuthSettings
	verifierFactory IDTokenVerifierFactory
}

// NewAuthorizer captures a settings snapshot. It never reads global
// configuration, so a new authorizer is required to change behaviour.
func NewAuthorizer(settings AuthSettings, options ...AuthorizerOption) (*Authorizer, error) {
	authorizer := &Authorizer{settings: settings.clone()}
	for _, option := range options {
		option(authorizer)
	}

	if authorizer.settings.LocalJWT.Enabled {
		if _, _, err := authorizer.settings.LocalJWT.signingMethod(); err != nil {
			return nil, err
		}
	}
	if authorizer.settings.OAuth.Enabled && authorizer.verifierFactory == nil {
		return nil, errors.New("oauth is enabled but no id token verifier factory was provided")
	}
	return authorizer, nil
}

// clone makes the settings snapshot independent of the caller's maps. Strings
// and booleans are already copied by value.
func (s AuthSettings) clone() AuthSettings {
	s.Basic.AllowedUsers = maps.Clone(s.Basic.AllowedUsers)
	return s
}

// ResolveAccessRule validates a surface's declared strategy and roles against
// the settings snapshot.
func (a *Authorizer) ResolveAccessRule(strategy AuthStrategy, requiredRoles []string) (AccessRule, error) {
	if len(requiredRoles) > 0 && (strategy == AuthNone || strategy == AuthBasic) {
		return AccessRule{}, fmt.Errorf("%w: strategy %q does not support required roles", ErrInvalidAccessRule, strategy)
	}

	switch strategy {
	case AuthNone:
		return AccessRule{public: true}, nil
	case AuthLocalJWT:
		if !a.settings.LocalJWT.Enabled {
			return AccessRule{}, fmt.Errorf("%w: strategy %q is disabled", ErrInvalidAccessRule, strategy)
		}
	case AuthOAuth:
		if !a.settings.OAuth.Enabled {
			return AccessRule{}, fmt.Errorf("%w: strategy %q is disabled", ErrInvalidAccessRule, strategy)
		}
	case AuthBasic:
		if !a.settings.Basic.Enabled {
			return AccessRule{}, fmt.Errorf("%w: strategy %q is disabled", ErrInvalidAccessRule, strategy)
		}
	case AuthLocalOrOIDC:
		if !a.settings.LocalJWT.Enabled || !a.settings.OAuth.Enabled {
			return AccessRule{}, fmt.Errorf("%w: strategy %q requires local jwt and oauth to be enabled", ErrInvalidAccessRule, strategy)
		}
	default:
		return AccessRule{}, fmt.Errorf("%w: unknown strategy %q", ErrInvalidAccessRule, strategy)
	}

	return AccessRule{
		strategy:      strategy,
		requiredRoles: append([]string(nil), requiredRoles...),
	}, nil
}

// Authorize proves the caller's identity and stores exactly one Identity in the
// returned context.
func (a *Authorizer) Authorize(ctx context.Context, rule AccessRule, credential Credential) (context.Context, error) {
	if rule.public {
		return ctx, nil
	}

	if rule.strategy == AuthLocalOrOIDC {
		return a.authorizeFirstFullyAuthorized(ctx, rule, credential)
	}

	identity, err := a.verify(ctx, rule.strategy, credential)
	if err != nil {
		if errors.Is(err, ErrCredentialNotApplicable) {
			return ctx, ErrAuthenticationRequired
		}
		return ctx, err
	}
	if !hasAnyRole(rule.requiredRoles, identity.Roles) {
		return ctx, ErrForbidden
	}
	return withIdentity(ctx, identity), nil
}

// authorizeFirstFullyAuthorized tries each strategy in order and accepts the
// first credential that verifies and satisfies the required roles. Identities
// from different verifiers are never merged.
func (a *Authorizer) authorizeFirstFullyAuthorized(ctx context.Context, rule AccessRule, credential Credential) (context.Context, error) {
	var sawDefiniteRejection, sawUnavailable, sawRoleDenial bool

	for _, strategy := range []AuthStrategy{AuthLocalJWT, AuthOAuth} {
		identity, err := a.verify(ctx, strategy, credential)
		switch {
		case errors.Is(err, ErrCredentialNotApplicable):
			continue
		case errors.Is(err, ErrUnavailable):
			sawUnavailable = true
			continue
		case err != nil:
			sawDefiniteRejection = true
			continue
		}

		if !hasAnyRole(rule.requiredRoles, identity.Roles) {
			sawRoleDenial = true
			continue
		}
		return withIdentity(ctx, identity), nil
	}

	switch {
	case sawRoleDenial:
		return ctx, ErrForbidden
	case sawDefiniteRejection:
		return ctx, ErrAuthenticationRequired
	case sawUnavailable:
		return ctx, ErrUnavailable
	default:
		return ctx, ErrAuthenticationRequired
	}
}

func (a *Authorizer) verify(ctx context.Context, strategy AuthStrategy, credential Credential) (Identity, error) {
	switch strategy {
	case AuthLocalJWT:
		return a.verifyLocalJWT(credential)
	case AuthOAuth:
		return a.verifyIDToken(ctx, credential)
	case AuthBasic:
		return a.verifyBasic(credential)
	default:
		return Identity{}, ErrInvalidAccessRule
	}
}

// hasAnyRole applies required-role semantics: any one required role, matched
// exactly, is enough. An empty requirement passes.
func hasAnyRole(required, granted []string) bool {
	if len(required) == 0 {
		return true
	}
	for _, needed := range required {
		for _, held := range granted {
			if needed == held {
				return true
			}
		}
	}
	return false
}

// verifyBasic checks a basic credential against the snapshot's bcrypt users.
func (a *Authorizer) verifyBasic(credential Credential) (Identity, error) {
	if credential.kind != credentialBasic {
		return Identity{}, ErrCredentialNotApplicable
	}

	allowedPassword, found := a.settings.Basic.AllowedUsers[credential.username]
	if !found || allowedPassword == "" {
		return Identity{}, fmt.Errorf("%w: unknown basic user", ErrAuthenticationRequired)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(allowedPassword), []byte(credential.password)); err != nil {
		return Identity{}, fmt.Errorf("%w: invalid basic password", ErrAuthenticationRequired)
	}

	return Identity{Username: credential.username}, nil
}

// verifyIDToken verifies a bearer token through the injected verifier factory.
func (a *Authorizer) verifyIDToken(ctx context.Context, credential Credential) (Identity, error) {
	if credential.kind != credentialBearer || credential.bearerToken == "" {
		return Identity{}, ErrCredentialNotApplicable
	}
	if a.verifierFactory == nil {
		// NewAuthorizer rejects this configuration, so it is unreachable for a
		// constructed authorizer.
		return Identity{}, fmt.Errorf("%w: no id token verifier is configured", ErrAuthenticationRequired)
	}

	verifier, err := a.verifierFactory(ctx)
	if err != nil {
		if errors.Is(err, ErrCredentialNotApplicable) {
			return Identity{}, ErrCredentialNotApplicable
		}
		return Identity{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	claims, err := verifier.Verify(ctx, credential.bearerToken)
	if err != nil {
		if errors.Is(err, ErrCredentialNotApplicable) {
			return Identity{}, ErrCredentialNotApplicable
		}
		if errors.Is(err, ErrUnavailable) {
			return Identity{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return Identity{}, fmt.Errorf("%w: invalid id token", ErrAuthenticationRequired)
	}
	if len(claims) == 0 {
		return Identity{}, fmt.Errorf("%w: id token has no claims", ErrAuthenticationRequired)
	}

	roles, _ := rolesFromClaim(claims, a.settings.OAuth.RolesClaim)
	return identityFromClaims(claims, roles, credential.bearerToken), nil
}

func identityFromClaims(claims map[string]interface{}, roles []string, rawToken string) Identity {
	identity := Identity{
		Roles:       roles,
		Claims:      claims,
		ClientToken: rawToken,
	}
	if issuer, ok := claims["iss"].(string); ok {
		identity.Issuer = issuer
	}
	if username, ok := claims["preferred_username"].(string); ok {
		identity.Username = username
	}
	return identity
}

func withIdentity(ctx context.Context, identity Identity) context.Context {
	identity.Roles = slices.Clone(identity.Roles)
	identity.Claims = maps.Clone(identity.Claims)
	return context.WithValue(ctx, identityContextKey{}, identity)
}

func (s LocalJWTSettings) signingMethod() (jwt.SigningMethod, []byte, error) {
	if s.SigningKey == "" {
		return nil, nil, errors.New("local jwt signing key is not configured")
	}

	switch s.SigningMethod {
	case "HS256":
		return jwt.SigningMethodHS256, []byte(s.SigningKey), nil
	case "HS384":
		return jwt.SigningMethodHS384, []byte(s.SigningKey), nil
	case "HS512":
		return jwt.SigningMethodHS512, []byte(s.SigningKey), nil
	default:
		return nil, nil, fmt.Errorf("local jwt signing method %q is not configured or invalid", s.SigningMethod)
	}
}

// verifyLocalJWT verifies a bearer token against the settings snapshot. Tokens
// that are not signed with the configured method are not applicable rather than
// rejected, so a local-or-oidc rule can fall through to the ID token verifier.
func (a *Authorizer) verifyLocalJWT(credential Credential) (Identity, error) {
	if credential.kind != credentialBearer || credential.bearerToken == "" {
		return Identity{}, ErrCredentialNotApplicable
	}

	method, key, err := a.settings.LocalJWT.signingMethod()
	if err != nil {
		// NewAuthorizer rejects this configuration, so it is unreachable for a
		// constructed authorizer.
		return Identity{}, fmt.Errorf("%w: %v", ErrAuthenticationRequired, err)
	}

	unverified, _, err := new(jwt.Parser).ParseUnverified(credential.bearerToken, jwt.MapClaims{})
	if err != nil || unverified == nil || unverified.Method == nil {
		return Identity{}, ErrCredentialNotApplicable
	}
	if unverified.Method.Alg() != method.Alg() {
		return Identity{}, ErrCredentialNotApplicable
	}

	token, err := jwt.Parse(
		credential.bearerToken,
		func(*jwt.Token) (interface{}, error) { return key, nil },
		jwt.WithValidMethods([]string{method.Alg()}),
	)
	if err != nil || !token.Valid {
		return Identity{}, fmt.Errorf("%w: invalid local jwt", ErrAuthenticationRequired)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || len(claims) == 0 {
		return Identity{}, fmt.Errorf("%w: local jwt has no claims", ErrAuthenticationRequired)
	}

	roles, present := rolesFromClaim(claims, a.settings.LocalJWT.RolesClaim)
	if !present {
		if role, ok := claims["role"].(string); ok {
			roles = []string{role}
		}
	}

	return identityFromClaims(claims, roles, credential.bearerToken), nil
}

// rolesFromClaim reads roles from a literal or dotted claim path. The boolean
// reports whether the claim was present, which distinguishes an absent claim
// from one that yielded no roles.
func rolesFromClaim(claims map[string]interface{}, claimPath string) ([]string, bool) {
	if claimPath == "" {
		claimPath = "roles"
	}

	raw, present := claimValue(claims, claimPath)
	if !present {
		return nil, false
	}

	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...), true
	case []interface{}:
		roles := make([]string, 0, len(typed))
		for _, item := range typed {
			if role, ok := item.(string); ok {
				roles = append(roles, role)
			}
		}
		return roles, true
	case map[string]interface{}:
		roles := make([]string, 0, len(typed))
		for role := range typed {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		return roles, true
	default:
		return nil, true
	}
}

func claimValue(claims map[string]interface{}, claimPath string) (interface{}, bool) {
	if value, ok := claims[claimPath]; ok {
		return value, true
	}

	var current interface{} = claims
	for _, part := range strings.Split(claimPath, ".") {
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
