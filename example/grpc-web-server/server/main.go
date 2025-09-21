package main

import (
	"context"
	"embed"

	"grpc-server/gen"

	p "github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

//go:embed ui/*
var uiAssets embed.FS

type Server struct {
	gen.UnimplementedHelloServiceServer
}

func (s *Server) Register(server *grpc.Server) {
	gen.RegisterHelloServiceServer(server, s)
}

func (s *Server) SayHello(ctx context.Context, req *gen.HelloRequest) (*gen.HelloResponse, error) {

	p.Log.Info("Received SayHello request", zap.String("name", req.Name))
	return &gen.HelloResponse{Result: "Hello " + req.Name}, nil
}

func main() {

	config := p.Config{}
	config.Component.ComponentName = "grpc-auth"
	config.Grpc.Server.ListeningAddress = "127.0.0.1:9001"
	config.Grpc.Server.TLSCertFileName = "server.crt"
	config.Grpc.Server.TLSKeyFileName = "server.key"
	config.Grpc.Server.TLSEnabled = true

	p.SetPlatformConfiguration(config)

	services := []p.GRPCService{
		&Server{},
	}

	p.StartGrpcServerWithWeb(services, "ui", &uiAssets)
}
