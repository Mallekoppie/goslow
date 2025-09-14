package main

import (
	"context"
	"log"
	"net"
	"strings"

	"grpc-server/gen"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
)

type Server struct {
	gen.UnimplementedHelloServiceServer
}

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

func main() {

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
		grpc.ChainUnaryInterceptor(authInterceptor),
		// grpc.UnaryInterceptor(authInterceptor),
	}

	grpcServer := grpc.NewServer(serverOptions...)
	// grpcServer := grpc.NewServer()
	gen.RegisterHelloServiceServer(grpcServer, &Server{})

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
		platform.Logger.Error("failed to serve", zap.Error(err))
	}

}

func valid(authorization []string) bool {
	if len(authorization) < 1 {
		return false
	}
	token := strings.TrimPrefix(authorization[0], "Bearer ")
	// Perform the token validation here. For the sake of this example, the code
	// here forgoes any of the usual OAuth2 token validation and instead checks
	// for a token matching an arbitrary string.
	claims, err := platform.LocalJwt.ValidateLocalJwtToken(token)
	if err != nil {
		platform.Logger.Error("failed to validate token", zap.Error(err))
		return false
	}
	return claims["username"] == "user"
}

func authInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	if info.FullMethod == gen.HelloService_Login_FullMethodName {
		platform.Logger.Info("Skipping auth for Login")
		return handler(ctx, req)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errMissingMetadata
	}
	// The keys within metadata.MD are normalized to lowercase.
	// See: https://godoc.org/google.golang.org/grpc/metadata#New
	if !valid(md["authorization"]) {
		platform.Logger.Error("invalid token", zap.Strings("authorization", md["authorization"]))
		return gen.LoginResponse{
			Token:   "",
			Message: "invalid token",
			Success: false,
		}, errInvalidToken
	}
	platform.Logger.Info("Valid token", zap.Strings("authorization", md["authorization"]))
	// Continue execution of handler after ensuring a valid token.
	return handler(ctx, req)
}
