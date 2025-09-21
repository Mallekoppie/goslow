package main

import (
	"context"

	"grpc-server/gen"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) SayHello(ctx context.Context, req *gen.HelloRequest) (*gen.HelloResponse, error) {
	platform.Logger.Info("Received SayHello request", zap.String("name", req.Name))
	return &gen.HelloResponse{Result: "Hello " + req.Name}, nil
}

// Implement grpc Login function
func (s *Server) Login(ctx context.Context, req *gen.LoginRequest) (*gen.LoginResponse, error) {
	platform.Logger.Info("Received Login request", zap.String("username", req.Username))
	if req.Username == "user" && req.Password == "pass" {
		token, err := platform.LocalJwt.NewLocalJwtToken(map[string]interface{}{
			"username": req.Username,
		})
		if err != nil {
			platform.Logger.Error("failed to create token", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to create token")
		}

		platform.Logger.Info("Login successful", zap.String("user", req.Username))

		return &gen.LoginResponse{Token: token, Success: true, Message: "login successful"}, nil
	}
	return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
}

type Server struct {
	gen.UnimplementedHelloServiceServer
}

func (s *Server) Register(server *grpc.Server) {
	gen.RegisterHelloServiceServer(server, s)
}

func main() {

	config := platform.Config{}
	config.Component.ComponentName = "grpc-auth"
	config.Grpc.Server.ListeningAddress = "127.0.0.1:9001"
	config.Grpc.Server.TLSCertFileName = "server.crt"
	config.Grpc.Server.TLSKeyFileName = "server.key"
	config.Grpc.Server.TLSEnabled = true
	config.Grpc.Server.UnAuthenticatedPaths = []string{"/gen.HelloService/Login"}
	config.Auth.Server.LocalJwt.Enabled = true
	config.Auth.Server.LocalJwt.JwtSigningKey = "blablablacksheep"
	config.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	config.Auth.Server.LocalJwt.JwtExpiration = 60

	platform.SetPlatformConfiguration(config)

	services := []platform.GRPCService{
		&Server{},
	}

	platform.StartGrpcServer(services)
}
