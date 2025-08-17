package main

import (
	"context"
	"log"
	"net"

	"grpc-server/gen"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type Server struct {
	gen.UnimplementedHelloServiceServer
}

func (s *Server) SayHello(ctx context.Context, req *gen.HelloRequest) (*gen.HelloResponse, error) {
	platform.Logger.Info("Received SayHello request", zap.String("name", req.Name))
	return &gen.HelloResponse{Result: "Hello " + req.Name}, nil
}

func main() {

	lis, err := net.Listen("tcp", "localhost:9001")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
		platform.Logger.Error("failed to listen", zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	gen.RegisterHelloServiceServer(grpcServer, &Server{})

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
		platform.Logger.Error("failed to serve", zap.Error(err))
	}

}
