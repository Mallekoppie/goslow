package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Mallekoppie/goslow/platform"
	jwt "github.com/golang-jwt/jwt/v5"
)

const testSigningKey = "my super secret key"

func authorizedQueryRequest(t *testing.T) *http.Request {
	t.Helper()

	claims := jwt.MapClaims{
		"username": "test",
		"roles":    []string{"user"},
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSigningKey))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}

	authorizer, err := platform.NewAuthorizer(platform.AuthSettings{
		LocalJWT: platform.LocalJWTSettings{
			Enabled:       true,
			SigningKey:    testSigningKey,
			SigningMethod: "HS256",
			RolesClaim:    "roles",
		},
	})
	if err != nil {
		t.Fatalf("build authorizer: %v", err)
	}

	rule, err := authorizer.ResolveAccessRule(platform.AuthLocalJWT, nil)
	if err != nil {
		t.Fatalf("resolve access rule: %v", err)
	}

	ctx, err := authorizer.Authorize(context.Background(), rule, platform.BearerCredential(token))
	if err != nil {
		t.Fatalf("authorize test credential: %v", err)
	}

	return httptest.NewRequest(http.MethodGet, "/query", nil).WithContext(ctx)
}

func TestHandleQueryRejectsRequestWithoutIdentity(t *testing.T) {
	recorder := httptest.NewRecorder()
	HandleQuery(recorder, httptest.NewRequest(http.MethodGet, "/query", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if strings.Contains(recorder.Body.String(), "successful") {
		t.Fatalf("body %q reports success for an unauthenticated caller", recorder.Body.String())
	}
}

func TestHandleQueryDescribesAuthorizedCaller(t *testing.T) {
	recorder := httptest.NewRecorder()
	HandleQuery(recorder, authorizedQueryRequest(t))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "test") {
		t.Fatalf("body %q does not describe the caller", recorder.Body.String())
	}
}
