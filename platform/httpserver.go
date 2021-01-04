package platform

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func newRouter(serviceRoutes Routes) (*mux.Router, error) {
	router := mux.NewRouter().StrictSlash(true)

	conf, err := getPlatformConfiguration()
	if err != nil {
		return nil, err
	}

	for index := range serviceRoutes {
		route := serviceRoutes[index]
		var handler http.Handler
		handler = route.HandlerFunc

		// Add the middleware components. The are executed from the bottom up
		// handler = middleware.AllowedContentType(handler, route.AllowedContentType)

		// TODO: Check if enabled before adding these
		handler = loggingMiddleware(handler, route.SlaMs)

		if conf.Auth.Server.Basic.Enabled {
			handler = basicAuthMiddleware(handler, conf.Auth.Server.Basic.AllowedUsers)
		}

		if conf.Auth.Server.OAuth.Enabled {
			handler = oAuth2Middleware(handler, route.RolesRequired)
		}

		if conf.HTTP.Server.AllowCorsForLocalDevelopment == true {
			handler = AllowCorsForLocalDevelopment(handler)
		}

		router.
			Path(route.Path).
			Methods(route.Method).
			Handler(handler)

	}

	return router, nil
}

func StartHttpServer(routes Routes) {
	config, err := getPlatformConfiguration()
	if err != nil {
		Logger.Error("Error reading platform configuration", zap.Error(err))
		return
	}

	router, err := newRouter(routes)
	if err != nil {
		Logger.Fatal("Error starting HTTP server", zap.Error(err))
		return
	}

	// Do this for each database we add
	defer Database.BoltDb.Close()

	Logger.Info("Starting new HTTP server", zap.String("ListeingAddress", config.HTTP.Server.ListeningAddress))
	if config.HTTP.Server.TLSEnabled {
		Logger.Error("TLS Server stopped: ", zap.Error(
			http.ListenAndServeTLS(config.HTTP.Server.ListeningAddress,
				config.HTTP.Server.TLSCertFileName,
				config.HTTP.Server.TLSKeyFileName,
				router)))
	} else {
		Logger.Error("HTTP Server stopped: ", zap.Error(
			http.ListenAndServe(config.HTTP.Server.ListeningAddress, router)))
	}
}

type Route struct {
	Path               string
	Method             string
	HandlerFunc        http.HandlerFunc
	SlaMs              int64
	RolesRequired      []string
	AllowedContentType string
}

type Routes []Route
