package service

import (
	"net/http"
)

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	// w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello World"))
}
