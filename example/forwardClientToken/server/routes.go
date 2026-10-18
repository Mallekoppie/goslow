package main

import (
	"net/http"

	"github.com/Mallekoppie/goslow/example/forwardClientToken/server/service"

	"github.com/Mallekoppie/goslow/platform"
)

var Routes = platform.Routes{
	platform.Route{
		Path:          "/",
		Method:        http.MethodGet,
		HandlerFunc:   service.HelloWorld,
		SlaMs:         0,
		RolesRequired: []string{"user"},
		Auth:          platform.AuthOAuth,
	},
}
