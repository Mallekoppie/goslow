package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"

	"grpc-server/gen"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

//go:embed ui/*
var uiAssets embed.FS

type Server struct {
	gen.UnimplementedHelloServiceServer
}

func (s *Server) SayHello(ctx context.Context, req *gen.HelloRequest) (*gen.HelloResponse, error) {
	platform.Logger.Info("Received SayHello request", zap.String("name", req.Name))
	return &gen.HelloResponse{Result: "Hello " + req.Name}, nil
}

func main() {
	uiFs, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		log.Fatalf("failed to create sub filesystem: %v", err)
		platform.Logger.Error("failed to create sub filesystem", zap.Error(err))
	}
	uiHandler := http.FileServer(http.FS(uiFs))

	lis, err := net.Listen("tcp", "localhost:9001")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
		platform.Logger.Error("failed to listen", zap.Error(err))
	}

	creds, err := credentials.NewServerTLSFromFile("server.crt", "server.key")
	if err != nil {
		log.Fatalf("failed to load TLS credentials: %v", err)
		platform.Logger.Error("failed to load TLS credentials", zap.Error(err))
	}

	serverOptions := []grpc.ServerOption{
		grpc.Creds(creds),
	}

	grpcServer := grpc.NewServer(serverOptions...)
	gen.RegisterHelloServiceServer(grpcServer, &Server{})

	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route gRPC requests to grpcServer
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}
		// Route everything else to UI
		uiHandler.ServeHTTP(w, r)
	})

	if err := http.ServeTLS(lis, mux, "server.crt", "server.key"); err != nil {
		log.Fatalf("failed to serve: %v", err)
		platform.Logger.Error("failed to serve", zap.Error(err))
	}
}
