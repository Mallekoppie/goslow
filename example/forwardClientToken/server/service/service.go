package service

import (
	"net/http"

	"go.uber.org/zap"

	p "github.com/Mallekoppie/goslow/platform"
)

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	p.Log.Info("We arrived at a new world!!!!")

	clientToken := r.Header.Get("X-Client-Token")

	if len(clientToken) < 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, claims, err := p.OAuth.ValidateClientToken(clientToken)
	if err != nil {
		p.Log.Error("Client token validation failed", zap.Error(err))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	p.Log.Info("Received valid token", zap.String("username", claims["preferred_username"].(string)))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello World"))
}
