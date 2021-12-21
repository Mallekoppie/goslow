package service

import (
	"github.com/Mallekoppie/goslow/example/forwardClientToken/client/logic"
	"net/http"

	"github.com/Mallekoppie/goslow/platform"
)

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	platform.Logger.Info("We arrived at a new world!!!!")

	clientToken := r.Context().Value(platform.ContextOAuthClientToken).(string)

	if len(clientToken) < 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := logic.CallServer(clientToken)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello World"))
}
