package main

import (
	"net/http"

	"github.com/Mallekoppie/goslow/example/service"
	"github.com/Mallekoppie/goslow/platform"
)

var Routes = platform.Routes{
	platform.Route{
		Path:        "/",
		Method:      http.MethodGet,
		HandlerFunc: service.HelloWorld,
		SlaMs:       0,
	},
}
