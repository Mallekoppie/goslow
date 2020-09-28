package platform

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func newRouter(serviceRoutes Routes) (*mux.Router, error) {
	router := mux.NewRouter().StrictSlash(true)

	conf, err := readPlatformConfiguration()
	if err != nil {
		return nil, err
	}

	for index := range serviceRoutes {
		route := serviceRoutes[index]
		var handler http.Handler
		handler = route.HandlerFunc

		// Add the middleware components. The are executed from the bottom up
		// handler = middleware.AllowedContentType(handler, route.AllowedContentType)
		// handler = middleware.AllowCors(handler)

		// TODO: Check if enabled before adding these
		handler = serviceMethodSlaMiddleware(handler, route.SlaMs)
		if conf.Auth.Server.OAuth.Enabled {
			handler = oAuth2Middleware(handler, route.RolesRequired) // Disabled during development
		}

		router.
			Path(route.Path).
			Methods(route.Method).
			Handler(handler)

	}

	router.Use(loggingMiddleware)

	return router, nil
}

func StartHttpServer(routes Routes) {
	config, err := readPlatformConfiguration()
	if err != nil {
		log.Println("Error reading configuration: ", err.Error())
	}

	router, err := newRouter(routes)
	if err != nil {
		log.Fatalln("Error starting HTTP server: ", err.Error())
		return
	}

	if config.HTTP.Server.TLSEnabled {
		log.Println("TLS Server stopped: ",
			http.ListenAndServeTLS(config.HTTP.Server.ListeningAddress,
				config.HTTP.Server.TLSCertFileName,
				config.HTTP.Server.TLSKeyFileName,
				router))
	} else {
		log.Println("HTTP Server stopped: ",
			http.ListenAndServe(config.HTTP.Server.ListeningAddress, router))
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
