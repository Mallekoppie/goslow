package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSecretsReturnsOnlyTheVaultErrorWhenVaultIsDisabled(t *testing.T) {
	recorder := httptest.NewRecorder()
	GetSecrets(recorder, httptest.NewRequest(http.MethodGet, "/secrets", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if body := recorder.Body.String(); body != "vault not enabled" {
		t.Fatalf("body = %q, want %q", body, "vault not enabled")
	}
}
