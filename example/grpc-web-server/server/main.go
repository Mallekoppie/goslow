package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"grpc-server/gen"
	"grpc-server/gen/genconnect"

	"connectrpc.com/connect"
	p "github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
)

//go:embed ui/*
var uiAssets embed.FS

// Server implements the generated Connect handler. The embedded Flutter assets
// and the RPC endpoints are served on the same origin.
type Server struct{}

func (s *Server) SayHello(ctx context.Context, request *connect.Request[gen.HelloRequest]) (*connect.Response[gen.HelloResponse], error) {
	p.Log.Info("Received SayHello request", zap.String("name", request.Msg.Name))
	return connect.NewResponse(&gen.HelloResponse{Result: "Hello " + request.Msg.Name}), nil
}

func (s *Server) Register(mux *http.ServeMux, options ...connect.HandlerOption) {
	prefix, handler := genconnect.NewHelloServiceHandler(s, options...)
	mux.Handle(prefix, handler)
}

func main() {
	config := p.Config{}
	config.Component.ComponentName = "grpc-web"
	config.Grpc.Server.ListeningAddress = "127.0.0.1:9001"
	config.Grpc.Server.TLSCertFileName = "server.crt"
	config.Grpc.Server.TLSKeyFileName = "server.key"
	config.Grpc.Server.TLSEnabled = true

	p.SetPlatformConfiguration(config)

	uiFS, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		p.Log.Error("failed to create sub filesystem for ui assets", zap.Error(err))
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	services := []p.ConnectService{
		&Server{},
	}

	err = p.StartConnectServerWithContext(ctx, p.ConnectServerOptions{
		Services:   services,
		WebHandler: http.FileServer(http.FS(uiFS)),
	})
	if err != nil {
		p.Log.Error("Connect server stopped", zap.Error(err))
	}
}
