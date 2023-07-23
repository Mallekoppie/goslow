package main

import (
	"github.com/Mallekoppie/goslow/platform"
	"http-web-server/service"
	"net/http"
)

var Routes = platform.Routes{
	platform.Route{
		Path:        "/db/write",
		Method:      http.MethodPost,
		HandlerFunc: service.WriteObject,
		SlaMs:       0,
	},
	platform.Route{
		Path:        "/db/read",
		Method:      http.MethodPost,
		HandlerFunc: service.ReadObject,
		SlaMs:       0,
	},
	platform.Route{
		Path:        "/config",
		Method:      http.MethodGet,
		HandlerFunc: service.GetConfiguration,
		SlaMs:       0,
	},
	platform.Route{
		Path:        "/all",
		Method:      http.MethodGet,
		HandlerFunc: service.ReadAll,
		SlaMs:       0,
	},
	platform.Route{
		Path:        "/secrets",
		Method:      http.MethodGet,
		HandlerFunc: service.GetSecrets,
		SlaMs:       0,
	},
	platform.Route{
		Path:          "/oauth2",
		Method:        http.MethodGet,
		HandlerFunc:   service.GetOAuthDetails,
		SlaMs:         0,
		RolesRequired: []string{"read"},
	},
}
