package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mallekoppie/goslow/platform"
)

type fakeIDTokenVerifier struct {
	claims map[string]interface{}
}

func (f fakeIDTokenVerifier) Verify(context.Context, string) (map[string]interface{}, error) {
	return f.claims, nil
}

// oauthRequest builds a request whose context carries an identity verified by
// the authorizer. The access rule carries no roles so the handler's own
// defence can be exercised in isolation.
func oauthRequest(t *testing.T, claims map[string]interface{}) *http.Request {
	t.Helper()

	authorizer, err := platform.NewAuthorizer(
		platform.AuthSettings{OAuth: platform.OAuthSettings{Enabled: true, RolesClaim: "roles"}},
		platform.WithIDTokenVerifierFactory(func(context.Context) (platform.IDTokenVerifier, error) {
			return fakeIDTokenVerifier{claims: claims}, nil
		}),
	)
	if err != nil {
		t.Fatalf("build authorizer: %v", err)
	}

	rule, err := authorizer.ResolveAccessRule(platform.AuthOAuth, nil)
	if err != nil {
		t.Fatalf("resolve access rule: %v", err)
	}

	ctx, err := authorizer.Authorize(context.Background(), rule, platform.BearerCredential("test-token"))
	if err != nil {
		t.Fatalf("authorize test credential: %v", err)
	}

	return httptest.NewRequest(http.MethodGet, "/oauth2", nil).WithContext(ctx)
}

func TestGetOAuthDetailsForbidsVerifiedCallerWithoutRequiredRole(t *testing.T) {
	request := oauthRequest(t, map[string]interface{}{
		"sub":                "caller-1",
		"preferred_username": "test",
		"roles":              []string{"other"},
	})

	recorder := httptest.NewRecorder()
	GetOAuthDetails(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestGetOAuthDetailsDescribesCallerWithRequiredRole(t *testing.T) {
	request := oauthRequest(t, map[string]interface{}{
		"sub":                "caller-1",
		"preferred_username": "test",
		"iss":                "https://idp.example",
		"roles":              []string{OAuthRequiredRole},
	})

	recorder := httptest.NewRecorder()
	GetOAuthDetails(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body struct {
		Username string   `json:"username"`
		Issuer   string   `json:"issuer"`
		Roles    []string `json:"roles"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	if body.Username != "test" {
		t.Fatalf("username = %q, want %q", body.Username, "test")
	}
	if body.Issuer != "https://idp.example" {
		t.Fatalf("issuer = %q, want %q", body.Issuer, "https://idp.example")
	}
	if len(body.Roles) != 1 || body.Roles[0] != OAuthRequiredRole {
		t.Fatalf("roles = %v, want [%q]", body.Roles, OAuthRequiredRole)
	}
}
