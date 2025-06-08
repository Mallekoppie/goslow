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
	},
	platform.Route{
		Path:         "/renew",
		Method:       http.MethodGet,
		HandlerFunc:  HandleRenew,
		SlaMs:        0,
		AuthRequired: true,
	},
	platform.Route{
		Path:         "/query",
		Method:       http.MethodGet,
		HandlerFunc:  HandleQuery,
		SlaMs:        0,
		AuthRequired: true,
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
	platform.Logger.Info("User logged in successfully", zap.String("username", login.Username))

}

func HandleQuery(w http.ResponseWriter, r *http.Request) {
	// Handle query logic here
	// For example, return user information based on JWT token

	ctx := r.Context()
	result := ctx.Value(platform.ContextLocalJwtClaims)
	if result == nil {
		platform.Logger.Error("No JWT claims found in context")
		w.Write([]byte("Query successful"))
		return
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		platform.Logger.Error("Invalid JWT claims type in context")
		http.Error(w, "Invalid JWT claims", http.StatusInternalServerError)
		return
	}
	username, ok := resultMap["username"].(string)
	if !ok {
		platform.Logger.Error("Username not found in JWT claims")
		http.Error(w, "Invalid JWT claims", http.StatusInternalServerError)
		return
	}
	platform.Logger.Info("Query successful", zap.String("username", username))
	w.Write([]byte("Query successful for user: " + username))

}

func HandleRenew(w http.ResponseWriter, r *http.Request) {
	// Handle token renewal logic here
	// For example, generate a new JWT token with updated claims

	ctx := r.Context()
	result := ctx.Value(platform.ContextLocalJwtClaims)
	if result == nil {
		http.Error(w, "No JWT claims found in context", http.StatusUnauthorized)
		return
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		http.Error(w, "Invalid JWT claims type in context", http.StatusInternalServerError)
		return
	}

	username, ok := resultMap["username"].(string)
	if !ok {
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
