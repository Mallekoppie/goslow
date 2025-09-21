package service

import (
	"net/http"

	"github.com/Mallekoppie/goslow/example/forwardClientToken/client/logic"

	p "github.com/Mallekoppie/goslow/platform"
)

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	p.Log.Info("We arrived at a new world!!!!")

	clientToken := r.Context().Value(p.ContextOAuthClientToken).(string)

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
