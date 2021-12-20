package service

import (
	"net/http"

	"github.com/Mallekoppie/goslow/platform"
)

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	platform.Logger.Info("We arrived at a new world!!!!")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello World"))
}
