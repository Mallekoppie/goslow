package service

import (
	"go.uber.org/zap"
	"net/http"

	"github.com/Mallekoppie/goslow/platform"
)

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	platform.Logger.Info("We arrived at a new world!!!!")

	clientToken := r.Header.Get("X-Client-Token")

	if len(clientToken) < 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, claims, err := platform.OAuth.ValidateClientToken(clientToken)
	if err != nil {
		platform.Logger.Error("Client token validation failed", zap.Error(err))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	platform.Logger.Info("Received valid token", zap.String("username", claims["preferred_username"].(string)))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello World"))
}
