package main

import (
	"net/http"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
)

func main() {
	platform.StartHttpServer(Routes)
}

var Routes = platform.Routes{
	platform.Route{
		Path:        "/login",
		Method:      http.MethodPost,
		HandlerFunc: HandleLogin,
		SlaMs:       0,
		Auth:        platform.AuthNone,
	},
	platform.Route{
		Path:        "/renew",
		Method:      http.MethodGet,
		HandlerFunc: HandleRenew,
		SlaMs:       0,
		Auth:        platform.AuthLocalJWT,
	},
	platform.Route{
		Path:        "/query",
		Method:      http.MethodGet,
		HandlerFunc: HandleQuery,
		SlaMs:       0,
		Auth:        platform.AuthLocalJWT,
	},
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Handle login logic here
	// For example, validate user credentials and generate JWT token
	login := LoginRequest{}
	platform.JsonMarshaller.ReadJsonRequest(r.Body, &login)
	if login.Username != "test" || login.Password != "test" {
		http.Error(w, "Invalid login credentials", http.StatusUnauthorized)
		return
	}

	token, err := platform.LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"username": login.Username,
		"roles":    []string{"user"},
	})

	if err != nil {
		http.Error(w, "Failed to create JWT token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/text")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(token))
	platform.Log.Info("User logged in successfully", zap.String("username", login.Username))

}

func HandleQuery(w http.ResponseWriter, r *http.Request) {
	// Handle query logic here
	// For example, return user information based on JWT token

	identity, ok := platform.IdentityFromContext(r.Context())
	if !ok {
		platform.Log.Error("No identity found in context")
		http.Error(w, "No identity found in context", http.StatusUnauthorized)
		return
	}

	username := identity.Username
	if username == "" {
		platform.Log.Error("Username not found in JWT claims")
		http.Error(w, "Invalid JWT claims", http.StatusInternalServerError)
		return
	}
	platform.Log.Info("Query successful", zap.String("username", username))
	w.Write([]byte("Query successful for user: " + username))

}

func HandleRenew(w http.ResponseWriter, r *http.Request) {
	// Handle token renewal logic here
	// For example, generate a new JWT token with updated claims

	identity, ok := platform.IdentityFromContext(r.Context())
	if !ok {
		http.Error(w, "No identity found in context", http.StatusUnauthorized)
		return
	}

	username := identity.Username
	if username == "" {
		http.Error(w, "Username not found in JWT claims", http.StatusInternalServerError)
		return
	}

	newToken, err := platform.LocalJwt.NewLocalJwtToken(map[string]interface{}{
		"username": username,
		"roles":    []string{"user"},
	})
	if err != nil {
		http.Error(w, "Failed to create new JWT token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/text")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(newToken))
}
