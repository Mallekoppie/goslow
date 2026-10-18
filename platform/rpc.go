package platform

import (
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// SPAOptions configures an index.html fallback for client-side routing.
type SPAOptions struct {
	// Assets is the embedded or on-disk filesystem containing the built app.
	Assets fs.FS
	// Index is the fallback document, usually "index.html".
	Index string
}

// rpcPathPattern matches Connect/gRPC procedure paths: /pkg.Service/Method.
var rpcPathPattern = regexp.MustCompile(`^/[^/]+\.[^/]+/[^/]+$`)

// isRPCRequest reports whether a request targets the RPC surface. RPC-shaped
// requests must never fall through to a web or SPA handler.
func isRPCRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.Header.Get("Connect-Protocol-Version") != "" {
		return true
	}

	switch baseContentType(r.Header.Get("Content-Type")) {
	case "application/grpc", "application/proto":
		return true
	}

	contentType := baseContentType(r.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "application/grpc+") ||
		strings.HasPrefix(contentType, "application/grpc-web") ||
		strings.HasPrefix(contentType, "application/connect+") {
		return true
	}

	return rpcPathPattern.MatchString(r.URL.Path)
}

func baseContentType(value string) string {
	if index := strings.IndexByte(value, ';'); index >= 0 {
		value = value[:index]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

// NewSPAHandler serves files from assets and falls back to the index document
// for non-asset, non-RPC paths so client-side routing works.
func NewSPAHandler(assets fs.FS, index string) http.Handler {
	if index == "" {
		index = "index.html"
	}

	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requested != "" && requested != "." {
			if file, err := assets.Open(requested); err == nil {
				_ = file.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		document, err := fs.ReadFile(assets, index)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(document)
	})
}

// withRPCAwareFallback routes RPC paths to the RPC mux, rejects unknown RPC
// requests instead of serving HTML, and sends everything else to the web
// handler when one is configured.
func withRPCAwareFallback(rpc *http.ServeMux, web http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := rpc.Handler(r); pattern != "" {
			rpc.ServeHTTP(w, r)
			return
		}

		if isRPCRequest(r) {
			http.Error(w, "unknown rpc procedure", http.StatusNotFound)
			return
		}

		if web != nil {
			web.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})
}
