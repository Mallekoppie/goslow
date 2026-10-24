package platform

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

func signLocalJWT(t *testing.T, key string, method jwt.SigningMethod, claims map[string]interface{}) string {
	t.Helper()
	token := jwt.NewWithClaims(method, jwt.MapClaims(claims))
	signed, err := token.SignedString([]byte(key))
	if err != nil {
		t.Fatalf("unable to sign local JWT: %v", err)
	}
	return signed
}

func TestAuthorizationOutcomeClassifiesEveryFailureCategory(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want AuthOutcome
	}{
		{name: "authentication required", err: ErrAuthenticationRequired, want: AuthOutcomeAuthenticationRequired},
		{name: "wrapped authentication required", err: fmt.Errorf("local jwt: %w", ErrAuthenticationRequired), want: AuthOutcomeAuthenticationRequired},
		{name: "forbidden", err: ErrForbidden, want: AuthOutcomeForbidden},
		{name: "wrapped forbidden", err: fmt.Errorf("required roles: %w", ErrForbidden), want: AuthOutcomeForbidden},
		{name: "unavailable", err: ErrUnavailable, want: AuthOutcomeUnavailable},
		{name: "wrapped unavailable", err: fmt.Errorf("provider: %w", ErrUnavailable), want: AuthOutcomeUnavailable},
		{name: "cancelled", err: ErrCancelled, want: AuthOutcomeCancelled},
		{name: "wrapped cancelled", err: fmt.Errorf("%w: %w", ErrCancelled, context.Canceled), want: AuthOutcomeCancelled},
		{name: "unclassified failure", err: errors.New("boom"), want: AuthOutcomeAuthenticationRequired},
		{name: "no failure", err: nil, want: AuthOutcomeAuthenticationRequired},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := AuthorizationOutcome(testCase.err); got != testCase.want {
				t.Fatalf("expected outcome %v, got %v", testCase.want, got)
			}
		})
	}
}

func localJWTSettings() LocalJWTSettings {
	return LocalJWTSettings{
		Enabled:       true,
		SigningKey:    "test-signing-key",
		SigningMethod: "HS256",
		RolesClaim:    "permissions",
	}
}

func stubIDTokenFactory() IDTokenVerifierFactory {
	return func(context.Context) (IDTokenVerifier, error) {
		return recordingIDTokenVerifier{}, nil
	}
}

func mustAuthorizer(t *testing.T, settings AuthSettings, options ...AuthorizerOption) *Authorizer {
	t.Helper()
	authorizer, err := NewAuthorizer(settings, options...)
	if err != nil {
		t.Fatalf("NewAuthorizer failed: %v", err)
	}
	return authorizer
}

func mustRule(t *testing.T, authorizer *Authorizer, strategy AuthStrategy, roles []string) AccessRule {
	t.Helper()
	rule, err := authorizer.ResolveAccessRule(strategy, roles)
	if err != nil {
		t.Fatalf("ResolveAccessRule failed: %v", err)
	}
	return rule
}

type recordingIDTokenVerifier struct {
	claims map[string]interface{}
	err    error
	calls  *int
}

func (v recordingIDTokenVerifier) Verify(ctx context.Context, rawToken string) (map[string]interface{}, error) {
	if v.calls != nil {
		*v.calls = *v.calls + 1
	}
	if v.err != nil {
		return nil, v.err
	}
	return v.claims, nil
}

func TestNewAuthorizerRejectsInvalidSettings(t *testing.T) {
	testCases := []struct {
		name     string
		settings AuthSettings
	}{
		{
			name:     "local jwt without a signing key",
			settings: AuthSettings{LocalJWT: LocalJWTSettings{Enabled: true, SigningMethod: "HS256"}},
		},
		{
			name:     "local jwt with an unsupported signing method",
			settings: AuthSettings{LocalJWT: LocalJWTSettings{Enabled: true, SigningKey: "key", SigningMethod: "RS256"}},
		},
		{
			name:     "oauth without a verifier factory",
			settings: AuthSettings{OAuth: OAuthSettings{Enabled: true}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewAuthorizer(testCase.settings); err == nil {
				t.Fatal("expected NewAuthorizer to reject the settings")
			}
		})
	}
}

func TestResolveAccessRuleRejectsInvalidPolicies(t *testing.T) {
	localJWT := LocalJWTSettings{Enabled: true, SigningKey: "secret", SigningMethod: "HS256"}
	oauth := OAuthSettings{Enabled: true}
	basic := BasicSettings{Enabled: true}

	testCases := []struct {
		name     string
		settings AuthSettings
		strategy AuthStrategy
		roles    []string
	}{
		{name: "none with required roles", strategy: AuthNone, roles: []string{"admin"}},
		{name: "basic with required roles", settings: AuthSettings{Basic: basic}, strategy: AuthBasic, roles: []string{"admin"}},
		{name: "basic disabled", strategy: AuthBasic},
		{name: "local jwt disabled", strategy: AuthLocalJWT},
		{name: "oauth disabled", strategy: AuthOAuth},
		{name: "local-or-oidc needs oauth", settings: AuthSettings{LocalJWT: localJWT}, strategy: AuthLocalOrOIDC},
		{name: "local-or-oidc needs local jwt", settings: AuthSettings{OAuth: oauth}, strategy: AuthLocalOrOIDC},
		{name: "unknown strategy", strategy: AuthStrategy("saml")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			authorizer := mustAuthorizer(t, testCase.settings, WithIDTokenVerifierFactory(stubIDTokenFactory()))
			if _, err := authorizer.ResolveAccessRule(testCase.strategy, testCase.roles); !errors.Is(err, ErrInvalidAccessRule) {
				t.Fatalf("expected ErrInvalidAccessRule, got %v", err)
			}
		})
	}
}

func TestResolveAccessRuleAcceptsEnabledStrategies(t *testing.T) {
	localJWT := LocalJWTSettings{Enabled: true, SigningKey: "secret", SigningMethod: "HS256"}
	oauth := OAuthSettings{Enabled: true}
	basic := BasicSettings{Enabled: true}

	testCases := []struct {
		name     string
		settings AuthSettings
		strategy AuthStrategy
		roles    []string
	}{
		{name: "public", strategy: AuthNone},
		{name: "local jwt", settings: AuthSettings{LocalJWT: localJWT}, strategy: AuthLocalJWT, roles: []string{"admin"}},
		{name: "oauth", settings: AuthSettings{OAuth: oauth}, strategy: AuthOAuth, roles: []string{"admin"}},
		{name: "basic", settings: AuthSettings{Basic: basic}, strategy: AuthBasic},
		{name: "local-or-oidc", settings: AuthSettings{LocalJWT: localJWT, OAuth: oauth}, strategy: AuthLocalOrOIDC, roles: []string{"admin"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			authorizer := mustAuthorizer(t, testCase.settings, WithIDTokenVerifierFactory(stubIDTokenFactory()))
			mustRule(t, authorizer, testCase.strategy, testCase.roles)
		})
	}
}

func TestAuthorizeLocalJWTReadsTheConfiguredRolesClaim(t *testing.T) {
	settings := AuthSettings{LocalJWT: localJWTSettings()}
	authorizer := mustAuthorizer(t, settings)
	rule := mustRule(t, authorizer, AuthLocalJWT, nil)

	token := signLocalJWT(t, settings.LocalJWT.SigningKey, jwt.SigningMethodHS256, map[string]interface{}{
		"permissions":        []interface{}{"reader", "writer"},
		"role":               "ignored",
		"sub":                "alice",
		"iss":                "local",
		"preferred_username": "alice",
	})

	ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential(token))
	if err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}

	identity, ok := IdentityFromContext(ctx)
	if !ok {
		t.Fatal("expected an identity in the returned context")
	}
	if !reflect.DeepEqual(identity.Roles, []string{"reader", "writer"}) {
		t.Fatalf("expected roles from the configured claim, got %v", identity.Roles)
	}
	if identity.Username != "alice" || identity.Issuer != "local" {
		t.Fatalf("expected username and issuer from claims, got %q / %q", identity.Username, identity.Issuer)
	}
	if identity.ClientToken != token {
		t.Fatal("expected the bearer token to be retained for forwarding")
	}
	if identity.Claims["sub"] != "alice" {
		t.Fatalf("expected claims in the identity, got %v", identity.Claims)
	}
}

func TestAuthorizeLocalJWTFallsBackToTheUsernameClaim(t *testing.T) {
	settings := AuthSettings{LocalJWT: localJWTSettings()}
	authorizer := mustAuthorizer(t, settings)
	rule := mustRule(t, authorizer, AuthLocalJWT, nil)

	token := signLocalJWT(t, settings.LocalJWT.SigningKey, jwt.SigningMethodHS256, map[string]interface{}{
		"username": "legacy-user",
		"roles":    []string{"user"},
	})

	ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential(token))
	if err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}

	identity, ok := IdentityFromContext(ctx)
	if !ok {
		t.Fatal("expected an identity in the returned context")
	}
	if identity.Username != "legacy-user" {
		t.Fatalf("expected the username claim when preferred_username is absent, got %q", identity.Username)
	}
}

func TestAuthorizeLocalJWTPrefersPreferredUsernameOverUsername(t *testing.T) {
	settings := AuthSettings{LocalJWT: localJWTSettings()}
	authorizer := mustAuthorizer(t, settings)
	rule := mustRule(t, authorizer, AuthLocalJWT, nil)

	token := signLocalJWT(t, settings.LocalJWT.SigningKey, jwt.SigningMethodHS256, map[string]interface{}{
		"preferred_username": "preferred",
		"username":           "legacy",
		"roles":              []string{"user"},
	})

	ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential(token))
	if err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}

	identity, ok := IdentityFromContext(ctx)
	if !ok {
		t.Fatal("expected an identity in the returned context")
	}
	if identity.Username != "preferred" {
		t.Fatalf("expected preferred_username to win, got %q", identity.Username)
	}
}

func TestAuthorizeLocalJWTUsesSingularRoleOnlyWhenTheConfiguredClaimIsAbsent(t *testing.T) {
	settings := AuthSettings{LocalJWT: localJWTSettings()}
	authorizer := mustAuthorizer(t, settings)
	rule := mustRule(t, authorizer, AuthLocalJWT, nil)

	testCases := []struct {
		name   string
		claims map[string]interface{}
		want   []string
	}{
		{
			name:   "configured claim absent falls back to role",
			claims: map[string]interface{}{"role": "admin"},
			want:   []string{"admin"},
		},
		{
			name:   "configured claim present does not fall back",
			claims: map[string]interface{}{"permissions": []interface{}{}, "role": "admin"},
			want:   []string{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			token := signLocalJWT(t, settings.LocalJWT.SigningKey, jwt.SigningMethodHS256, testCase.claims)
			ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential(token))
			if err != nil {
				t.Fatalf("Authorize failed: %v", err)
			}
			identity, _ := IdentityFromContext(ctx)
			if !reflect.DeepEqual(identity.Roles, testCase.want) {
				t.Fatalf("expected roles %v, got %v", testCase.want, identity.Roles)
			}
		})
	}
}

func TestAuthorizeLocalJWTRejectsAnInvalidSignature(t *testing.T) {
	settings := AuthSettings{LocalJWT: localJWTSettings()}
	authorizer := mustAuthorizer(t, settings)
	rule := mustRule(t, authorizer, AuthLocalJWT, nil)

	token := signLocalJWT(t, "a-different-key", jwt.SigningMethodHS256, map[string]interface{}{"sub": "mallory"})
	if _, err := authorizer.Authorize(context.Background(), rule, BearerCredential(token)); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("expected ErrAuthenticationRequired, got %v", err)
	}
}

func TestAuthorizeLocalJWTValidatesTokensAgainstTheSettingsSnapshot(t *testing.T) {
	settings := AuthSettings{LocalJWT: localJWTSettings()}
	authorizer := mustAuthorizer(t, settings)
	rule := mustRule(t, authorizer, AuthLocalJWT, nil)
	key := settings.LocalJWT.SigningKey

	testCases := []struct {
		name      string
		token     string
		wantError error
	}{
		{
			name:  "valid token",
			token: signLocalJWT(t, key, jwt.SigningMethodHS256, map[string]interface{}{"sub": "alice"}),
		},
		{
			name:      "expired token",
			token:     signLocalJWT(t, key, jwt.SigningMethodHS256, map[string]interface{}{"sub": "alice", "exp": time.Now().Add(-time.Hour).Unix()}),
			wantError: ErrAuthenticationRequired,
		},
		{
			name:      "foreign signature",
			token:     signLocalJWT(t, "a-different-key", jwt.SigningMethodHS256, map[string]interface{}{"sub": "mallory"}),
			wantError: ErrAuthenticationRequired,
		},
		{
			name:      "malformed token",
			token:     "not-a-token",
			wantError: ErrAuthenticationRequired,
		},
		{
			name:      "rejected signing method",
			token:     signLocalJWT(t, key, jwt.SigningMethodHS512, map[string]interface{}{"sub": "alice"}),
			wantError: ErrAuthenticationRequired,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential(testCase.token))
			if testCase.wantError != nil {
				if !errors.Is(err, testCase.wantError) {
					t.Fatalf("expected %v, got %v", testCase.wantError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Authorize failed: %v", err)
			}
			identity, ok := IdentityFromContext(ctx)
			if !ok {
				t.Fatal("expected an identity in the returned context")
			}
			if identity.Claims["sub"] != "alice" {
				t.Fatalf("expected the token claims in the identity, got %v", identity.Claims)
			}
		})
	}
}

func TestAuthorizeIDTokenUsesTheInjectedVerifierFactory(t *testing.T) {
	settings := AuthSettings{OAuth: OAuthSettings{Enabled: true, RolesClaim: "groups"}}
	factoryCalls, verifyCalls := 0, 0
	authorizer := mustAuthorizer(t, settings, WithIDTokenVerifierFactory(
		func(context.Context) (IDTokenVerifier, error) {
			factoryCalls++
			return recordingIDTokenVerifier{
				claims: map[string]interface{}{
					"groups":             []interface{}{"admin"},
					"preferred_username": "bob",
					"iss":                "https://idp.example",
					"sub":                "bob",
				},
				calls: &verifyCalls,
			}, nil
		},
	))
	rule := mustRule(t, authorizer, AuthOAuth, nil)

	ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential("id-token"))
	if err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}

	identity, ok := IdentityFromContext(ctx)
	if !ok {
		t.Fatal("expected an identity in the returned context")
	}
	if !reflect.DeepEqual(identity.Roles, []string{"admin"}) {
		t.Fatalf("expected roles from the OAuth roles claim, got %v", identity.Roles)
	}
	if identity.Username != "bob" || identity.Issuer != "https://idp.example" {
		t.Fatalf("expected username and issuer from claims, got %q / %q", identity.Username, identity.Issuer)
	}
	if identity.ClientToken != "id-token" {
		t.Fatal("expected the bearer token to be retained for forwarding")
	}
	if factoryCalls != 1 || verifyCalls != 1 {
		t.Fatalf("expected one factory and one verify call, got %d / %d", factoryCalls, verifyCalls)
	}
}

func TestAuthorizeBasicCredentialChecksTheSnapshotUsers(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	settings := AuthSettings{Basic: BasicSettings{
		Enabled:      true,
		AllowedUsers: map[string]string{"alice": string(hash)},
	}}
	authorizer := mustAuthorizer(t, settings)
	rule := mustRule(t, authorizer, AuthBasic, nil)

	ctx, err := authorizer.Authorize(context.Background(), rule, BasicCredential("alice", "s3cret"))
	if err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		t.Fatal("expected an identity in the returned context")
	}
	if identity.Username != "alice" {
		t.Fatalf("expected the authenticated username, got %q", identity.Username)
	}
	if len(identity.Roles) != 0 || identity.ClientToken != "" {
		t.Fatalf("basic auth must not grant roles or a client token, got %v / %q", identity.Roles, identity.ClientToken)
	}

	for _, credential := range []Credential{
		BasicCredential("alice", "wrong"),
		BasicCredential("mallory", "s3cret"),
	} {
		if _, err := authorizer.Authorize(context.Background(), rule, credential); !errors.Is(err, ErrAuthenticationRequired) {
			t.Fatalf("expected ErrAuthenticationRequired, got %v", err)
		}
	}
}

func TestAuthorizeEnforcesRequiredRolesAsAnyOfExact(t *testing.T) {
	settings := AuthSettings{LocalJWT: localJWTSettings()}
	authorizer := mustAuthorizer(t, settings)
	rule := mustRule(t, authorizer, AuthLocalJWT, []string{"admin", "reader"})

	testCases := []struct {
		name      string
		granted   []interface{}
		wantError error
	}{
		{name: "holds any required role", granted: []interface{}{"reader"}, wantError: nil},
		{name: "role matching is exact", granted: []interface{}{"Admin"}, wantError: ErrForbidden},
		{name: "holds no required role", granted: []interface{}{"writer"}, wantError: ErrForbidden},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			token := signLocalJWT(t, settings.LocalJWT.SigningKey, jwt.SigningMethodHS256, map[string]interface{}{
				"permissions": testCase.granted,
			})
			ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential(token))
			if !errors.Is(err, testCase.wantError) {
				t.Fatalf("expected error %v, got %v", testCase.wantError, err)
			}
			if testCase.wantError != nil {
				if _, ok := IdentityFromContext(ctx); ok {
					t.Fatal("a denied caller must not have an identity stored")
				}
			}
		})
	}
}

func TestAuthorizeRejectsMissingCredentials(t *testing.T) {
	settings := AuthSettings{
		LocalJWT: localJWTSettings(),
		OAuth:    OAuthSettings{Enabled: true},
		Basic:    BasicSettings{Enabled: true},
	}
	authorizer := mustAuthorizer(t, settings, WithIDTokenVerifierFactory(
		func(context.Context) (IDTokenVerifier, error) {
			t.Fatal("the identity provider must not be contacted without a credential")
			return nil, nil
		},
	))

	for _, strategy := range []AuthStrategy{AuthLocalJWT, AuthOAuth, AuthBasic, AuthLocalOrOIDC} {
		t.Run(string(strategy), func(t *testing.T) {
			rule := mustRule(t, authorizer, strategy, nil)
			if _, err := authorizer.Authorize(context.Background(), rule, Credential{}); !errors.Is(err, ErrAuthenticationRequired) {
				t.Fatalf("expected ErrAuthenticationRequired, got %v", err)
			}
		})
	}
}

func TestAuthorizeTreatsAVerifierDecliningARequestAsAuthenticationRequired(t *testing.T) {
	settings := AuthSettings{OAuth: OAuthSettings{Enabled: true}}
	authorizer := mustAuthorizer(t, settings, WithIDTokenVerifierFactory(
		func(context.Context) (IDTokenVerifier, error) {
			return recordingIDTokenVerifier{err: ErrCredentialNotApplicable}, nil
		},
	))
	rule := mustRule(t, authorizer, AuthOAuth, nil)

	if _, err := authorizer.Authorize(context.Background(), rule, BearerCredential("opaque")); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("expected ErrAuthenticationRequired, got %v", err)
	}
}

func TestAuthorizeLocalOrOIDCTakesTheFirstFullyAuthorizedCredential(t *testing.T) {
	settings := AuthSettings{
		LocalJWT: localJWTSettings(),
		OAuth:    OAuthSettings{Enabled: true, RolesClaim: "roles"},
	}
	factoryCalls := 0
	authorizer := mustAuthorizer(t, settings, WithIDTokenVerifierFactory(
		func(context.Context) (IDTokenVerifier, error) {
			factoryCalls++
			return recordingIDTokenVerifier{claims: map[string]interface{}{
				"roles": []interface{}{"admin"},
				"iss":   "https://idp.example",
			}}, nil
		},
	))
	rule := mustRule(t, authorizer, AuthLocalOrOIDC, []string{"admin"})

	token := signLocalJWT(t, settings.LocalJWT.SigningKey, jwt.SigningMethodHS256, map[string]interface{}{
		"permissions": []interface{}{"admin"},
		"iss":         "local",
	})
	ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential(token))
	if err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		t.Fatal("expected an identity in the returned context")
	}
	if identity.Issuer != "local" {
		t.Fatalf("expected the local identity to win, got issuer %q", identity.Issuer)
	}
	if factoryCalls != 0 {
		t.Fatalf("the ID token verifier must not run once a credential is fully authorized, ran %d times", factoryCalls)
	}
}

func TestAuthorizeLocalOrOIDCFallsThroughWithoutMergingIdentities(t *testing.T) {
	settings := AuthSettings{
		LocalJWT: localJWTSettings(),
		OAuth:    OAuthSettings{Enabled: true, RolesClaim: "roles"},
	}
	authorizer := mustAuthorizer(t, settings, WithIDTokenVerifierFactory(
		func(context.Context) (IDTokenVerifier, error) {
			return recordingIDTokenVerifier{claims: map[string]interface{}{
				"roles":              []interface{}{"admin"},
				"preferred_username": "bob",
				"iss":                "https://idp.example",
			}}, nil
		},
	))
	rule := mustRule(t, authorizer, AuthLocalOrOIDC, []string{"admin"})

	token := signLocalJWT(t, settings.LocalJWT.SigningKey, jwt.SigningMethodHS256, map[string]interface{}{
		"permissions": []interface{}{"reader"},
	})
	ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential(token))
	if err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}
	identity, _ := IdentityFromContext(ctx)
	if !reflect.DeepEqual(identity.Roles, []string{"admin"}) {
		t.Fatalf("expected only the ID token's roles, got %v", identity.Roles)
	}
	if identity.Username != "bob" || identity.Issuer != "https://idp.example" {
		t.Fatalf("expected the ID token identity, got %q / %q", identity.Username, identity.Issuer)
	}
}

func TestAuthorizeIDTokenReportsCancellationBeforeVerification(t *testing.T) {
	settings := AuthSettings{OAuth: OAuthSettings{Enabled: true}}
	verifierCalls := 0
	authorizer := mustAuthorizer(t, settings, WithIDTokenVerifierFactory(
		func(context.Context) (IDTokenVerifier, error) {
			verifierCalls++
			return recordingIDTokenVerifier{claims: map[string]interface{}{"roles": []interface{}{"admin"}}}, nil
		},
	))
	rule := mustRule(t, authorizer, AuthOAuth, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := authorizer.Authorize(ctx, rule, BearerCredential("token")); !errors.Is(err, ErrCancelled) {
		t.Fatalf("expected ErrCancelled, got %v", err)
	}
	if verifierCalls != 0 {
		t.Fatalf("a cancelled request must not build or call a verifier, ran %d times", verifierCalls)
	}
}

func TestAuthorizeLocalOrOIDCReportsCancellationDuringVerification(t *testing.T) {
	settings := AuthSettings{
		LocalJWT: localJWTSettings(),
		OAuth:    OAuthSettings{Enabled: true, RolesClaim: "roles"},
	}

	started := make(chan struct{})
	authorizer := mustAuthorizer(t, settings, WithIDTokenVerifierFactory(
		func(context.Context) (IDTokenVerifier, error) {
			return idTokenVerifierFunc(func(ctx context.Context, _ string) (map[string]interface{}, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			}), nil
		},
	))
	rule := mustRule(t, authorizer, AuthLocalOrOIDC, []string{"admin"})

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := authorizer.Authorize(ctx, rule, BearerCredential("opaque-token"))
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("token verification did not start")
	}
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("expected ErrCancelled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not stop verification")
	}
}

func TestAuthorizeLocalOrOIDCReportsTheAgreedFailureCategory(t *testing.T) {
	settings := AuthSettings{
		LocalJWT: localJWTSettings(),
		OAuth:    OAuthSettings{Enabled: true, RolesClaim: "roles"},
	}

	testCases := []struct {
		name      string
		token     string
		factory   IDTokenVerifierFactory
		roles     []string
		wantError error
	}{
		{
			name:  "a definite rejection outranks an outage",
			token: signLocalJWT(t, "a-different-key", jwt.SigningMethodHS256, map[string]interface{}{}),
			factory: func(context.Context) (IDTokenVerifier, error) {
				return nil, ErrUnavailable
			},
			wantError: ErrAuthenticationRequired,
		},
		{
			name:  "an outage with no definite rejection is unavailable",
			token: "opaque-token",
			factory: func(context.Context) (IDTokenVerifier, error) {
				return nil, ErrUnavailable
			},
			wantError: ErrUnavailable,
		},
		{
			name:  "a role denial is forbidden",
			token: signLocalJWT(t, settings.LocalJWT.SigningKey, jwt.SigningMethodHS256, map[string]interface{}{"permissions": []interface{}{"reader"}}),
			factory: func(context.Context) (IDTokenVerifier, error) {
				return nil, ErrUnavailable
			},
			roles:     []string{"admin"},
			wantError: ErrForbidden,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			authorizer := mustAuthorizer(t, settings, WithIDTokenVerifierFactory(testCase.factory))
			rule := mustRule(t, authorizer, AuthLocalOrOIDC, testCase.roles)
			if _, err := authorizer.Authorize(context.Background(), rule, BearerCredential(testCase.token)); !errors.Is(err, testCase.wantError) {
				t.Fatalf("expected %v, got %v", testCase.wantError, err)
			}
		})
	}
}

func TestNewAuthorizerCopiesItsSettingsSnapshot(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	users := map[string]string{"alice": string(hash)}
	settings := AuthSettings{
		LocalJWT: localJWTSettings(),
		OAuth:    OAuthSettings{Enabled: true, RolesClaim: "permissions"},
		Basic:    BasicSettings{Enabled: true, AllowedUsers: users},
	}
	authorizer := mustAuthorizer(t, settings, WithIDTokenVerifierFactory(
		func(context.Context) (IDTokenVerifier, error) {
			return recordingIDTokenVerifier{claims: map[string]interface{}{
				"permissions": []interface{}{"admin"},
			}}, nil
		},
	))

	// Mutating the caller's values must not change the captured snapshot.
	users["alice"] = "not-a-bcrypt-hash"
	settings.Basic.AllowedUsers = nil
	settings.LocalJWT.SigningKey = "mutated-key"
	settings.LocalJWT.RolesClaim = "mutated-claim"
	settings.OAuth.RolesClaim = "mutated-claim"

	localRule := mustRule(t, authorizer, AuthLocalJWT, []string{"admin"})
	token := signLocalJWT(t, "test-signing-key", jwt.SigningMethodHS256, map[string]interface{}{
		"permissions": []interface{}{"admin"},
	})
	if _, err := authorizer.Authorize(context.Background(), localRule, BearerCredential(token)); err != nil {
		t.Fatalf("expected the captured local JWT settings to remain in force, got %v", err)
	}

	basicRule := mustRule(t, authorizer, AuthBasic, nil)
	if _, err := authorizer.Authorize(context.Background(), basicRule, BasicCredential("alice", "s3cret")); err != nil {
		t.Fatalf("expected the captured basic users to remain in force, got %v", err)
	}

	oauthRule := mustRule(t, authorizer, AuthOAuth, []string{"admin"})
	if _, err := authorizer.Authorize(context.Background(), oauthRule, BearerCredential("id-token")); err != nil {
		t.Fatalf("expected the captured OAuth roles claim to remain in force, got %v", err)
	}
}

func TestAuthorizerNeverReadsGlobalConfigurationAtRequestTime(t *testing.T) {
	cfg := setupTestPlatformState(t)
	t.Cleanup(func() { resetPlatformState(t) })
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "global-key"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	SetPlatformConfiguration(*cfg)

	authorizer := mustAuthorizer(t, AuthSettings{LocalJWT: LocalJWTSettings{
		Enabled:       true,
		SigningKey:    "snapshot-key",
		SigningMethod: "HS256",
	}})
	rule := mustRule(t, authorizer, AuthLocalJWT, nil)

	snapshotToken := signLocalJWT(t, "snapshot-key", jwt.SigningMethodHS256, map[string]interface{}{"sub": "alice"})
	if _, err := authorizer.Authorize(context.Background(), rule, BearerCredential(snapshotToken)); err != nil {
		t.Fatalf("expected the snapshot key to be used, got %v", err)
	}

	globalToken := signLocalJWT(t, "global-key", jwt.SigningMethodHS256, map[string]interface{}{"sub": "alice"})
	if _, err := authorizer.Authorize(context.Background(), rule, BearerCredential(globalToken)); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("expected the global key to be ignored, got %v", err)
	}
}

func TestAuthSettingsFromConfigCopiesTheAuthSettings(t *testing.T) {
	cfg := &Config{}
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "key"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS512"
	cfg.Auth.Server.LocalJwt.RolesClaim = "permissions"
	cfg.Auth.Server.OAuth.Enabled = true
	cfg.Auth.Server.OAuth.IdpWellKnownURL = "https://idp.example/.well-known/openid-configuration"
	cfg.Auth.Server.OAuth.ClientID = "test-client"
	cfg.Auth.Server.OAuth.AllowedAlgorithms = []string{"RS256"}
	cfg.Auth.Server.OAuth.RolesClaim = "groups"
	cfg.Auth.Server.Basic.Enabled = true
	cfg.Auth.Server.Basic.AllowedUsers = map[string]string{"alice": "hash"}

	settings := AuthSettingsFromConfig(cfg)

	if settings.LocalJWT != (LocalJWTSettings{Enabled: true, SigningKey: "key", SigningMethod: "HS512", RolesClaim: "permissions"}) {
		t.Fatalf("unexpected local JWT settings: %+v", settings.LocalJWT)
	}
	wantOAuth := OAuthSettings{
		Enabled:           true,
		IdpWellKnownURL:   "https://idp.example/.well-known/openid-configuration",
		ClientID:          "test-client",
		AllowedAlgorithms: []string{"RS256"},
		RolesClaim:        "groups",
	}
	if !reflect.DeepEqual(settings.OAuth, wantOAuth) {
		t.Fatalf("unexpected OAuth settings: %+v", settings.OAuth)
	}
	if !reflect.DeepEqual(settings.Basic.AllowedUsers, map[string]string{"alice": "hash"}) {
		t.Fatalf("unexpected basic settings: %+v", settings.Basic)
	}

	cfg.Auth.Server.Basic.AllowedUsers["alice"] = "mutated"
	if settings.Basic.AllowedUsers["alice"] != "hash" {
		t.Fatal("expected the settings snapshot to own its basic users map")
	}
	cfg.Auth.Server.OAuth.AllowedAlgorithms[0] = "HS256"
	if settings.OAuth.AllowedAlgorithms[0] != "RS256" {
		t.Fatal("expected the settings snapshot to own its allowed algorithms")
	}
}

func TestPublicRuleAuthorizesWithoutCredentialOrIdentityProvider(t *testing.T) {
	factoryCalls := 0
	authorizer := mustAuthorizer(t, AuthSettings{}, WithIDTokenVerifierFactory(
		func(context.Context) (IDTokenVerifier, error) {
			factoryCalls++
			return recordingIDTokenVerifier{}, nil
		},
	))
	rule := mustRule(t, authorizer, AuthNone, nil)

	ctx, err := authorizer.Authorize(context.Background(), rule, BearerCredential("untouched"))
	if err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}
	if _, ok := IdentityFromContext(ctx); ok {
		t.Fatal("a public rule must not store an identity")
	}
	if factoryCalls != 0 {
		t.Fatalf("a public rule must not contact the identity provider, factory called %d times", factoryCalls)
	}
}
