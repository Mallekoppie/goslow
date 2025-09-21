package platform

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func newRouter(serviceRoutes Routes) (*mux.Router, error) {
	router := mux.NewRouter().StrictSlash(true)

	conf, err := GetPlatformConfiguration()
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

		if conf.Auth.Server.Basic.Enabled && route.AuthRequired {
			handler = basicAuthMiddleware(handler, conf.Auth.Server.Basic.AllowedUsers)
		}

		if conf.Auth.Server.OAuth.Enabled && route.AuthRequired {
			handler = oAuth2Middleware(handler, route.RolesRequired)
		}

		if conf.Auth.Server.LocalJwt.Enabled && route.AuthRequired {
			handler = localJwtAuthMiddleware(handler)
		}

		if conf.HTTP.Server.AllowCorsForLocalDevelopment {
			handler = AllowCorsForLocalDevelopment(handler)

			if route.Method != http.MethodOptions {
				router.
					Path(route.Path).
					Methods(http.MethodOptions).
					Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Add("Access-Control-Allow-Origin", "*")
						w.Header().Add("Access-Control-Allow-Methods", "*")
						w.Header().Add("Access-Control-Allow-Headers", "*")
					}))
			}

		}

		router.
			Path(route.Path).
			Methods(route.Method).
			Handler(handler)
	}

	return router, nil
}

func startHttpServerInternal(router *mux.Router) error {
	config, err := GetPlatformConfiguration()
	if err != nil {
		Log.Error("Error reading platform configuration", zap.Error(err))
		return err
	}

	// Do this for each database we add
	if config.Database.BoltDB.Enabled {
		defer Database.BoltDb.Close()
	}

	Log.Info("Starting new HTTP server", zap.String("ListeingAddress", config.HTTP.Server.ListeningAddress))
	if config.HTTP.Server.TLSEnabled {
		Log.Error("TLS Server stopped: ", zap.Error(
			http.ListenAndServeTLS(config.HTTP.Server.ListeningAddress,
				config.HTTP.Server.TLSCertFileName,
				config.HTTP.Server.TLSKeyFileName,
				router)))
	} else {
		Log.Error("HTTP Server stopped: ", zap.Error(
			http.ListenAndServe(config.HTTP.Server.ListeningAddress, router)))
	}

	return nil
}

func StartHttpServer(routes Routes) error {
	router, err := newRouter(routes)
	if err != nil {
		Log.Error("Error starting HTTP server", zap.Error(err))
		return err
	}

	return startHttpServerInternal(router)
}

// Deprecated: Use StartHttpServerWithWeb
func StartHttpServerWithHtmlHosting(routes Routes, dist embed.FS) error {
	router, err := newRouter(routes)
	if err != nil {
		Log.Error("Error starting HTTP server", zap.Error(err))
		return err
	}

	stripped, err := fs.Sub(dist, "dist")
	if err != nil {
		fmt.Println("Error stripping frontend")
	}
	fileServer := http.FileServer(http.FS(stripped))
	router.PathPrefix("/").Handler(fileServer)

	return startHttpServerInternal(router)
}

func StartHttpServerWithWeb(routes Routes, dist embed.FS) error {
	router, err := newRouter(routes)
	if err != nil {
		Log.Error("Error starting HTTP server", zap.Error(err))
		return err
	}

	stripped, err := fs.Sub(dist, "dist")
	if err != nil {
		fmt.Println("Error stripping frontend")
	}
	fileServer := http.FileServer(http.FS(stripped))
	router.PathPrefix("/").Handler(fileServer)

	return startHttpServerInternal(router)
}

type Route struct {
	Path               string
	Method             string
	HandlerFunc        http.HandlerFunc
	SlaMs              int64
	RolesRequired      []string
	AllowedContentType string
	AuthRequired       bool
}

type Routes []Route
