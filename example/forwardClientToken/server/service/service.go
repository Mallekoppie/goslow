package service

import (
	"errors"
	"net/http"
	"strconv"

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

	// The forwarded token was minted for the original client, so it is verified
	// without comparing its audience to this service's configured client ID.
	claims, err := p.OAuth.ValidateClientToken(r.Context(), clientToken)
	if err != nil {
		p.Log.Error("Client token validation failed", zap.Error(err))
		if errors.Is(err, p.ErrUnavailable) {
			w.Header().Set("Retry-After", strconv.Itoa(p.UnavailableRetryAfterSeconds))
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	username, _ := claims["preferred_username"].(string)
	p.Log.Info("Received valid token", zap.String("username", username))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello World"))
}
