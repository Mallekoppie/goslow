package platform

import (
	"context"
	"net"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	ErrGRPCMissingMetadata = status.Errorf(codes.InvalidArgument, "missing grpc auth metadata")
	ErrGRPCInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
)

type GRPCService interface {
	Register(server *grpc.Server)
}

func StartGrpcServer(services []GRPCService) {
	InitializeLogger()

	config, err := GetPlatformConfiguration()
	if err != nil {
		Logger.Fatal("Error reading platform configuration", zap.Error(err))
		return
	}

	conf := config.Grpc.Server
	Logger.Info("Starting new GRPC server", zap.String("ListeningAddress", conf.ListeningAddress))
	lis, err := net.Listen("tcp", conf.ListeningAddress)
	if err != nil {
		Logger.Error("failed to listen", zap.Error(err))
		return
	}

	serverOptions := []grpc.ServerOption{}

	if conf.TLSEnabled {
		creds, err := credentials.NewServerTLSFromFile(conf.TLSCertFileName, conf.TLSKeyFileName)
		if err != nil {
			Logger.Error("failed to load TLS credentials for GRPC server", zap.Error(err))
			return
		}

		serverOptions = append(serverOptions, grpc.Creds(creds))

		if config.Auth.Server.LocalJwt.Enabled {
			// Add JWT authentication interceptor
			serverOptions = append(serverOptions, grpc.UnaryInterceptor(grpcAuthInterceptor))
		}
	}

	grpcServer := grpc.NewServer(serverOptions...)

	for _, service := range services {
		service.Register(grpcServer)
	}

	if err := grpcServer.Serve(lis); err != nil {
		Logger.Error("failed to serve", zap.Error(err))
	}

}

func grpcAuthInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	if len(internalConfig.Grpc.Server.UnAuthenticatedPaths) > 0 {
		for _, path := range internalConfig.Grpc.Server.UnAuthenticatedPaths {
			if path == info.FullMethod {
				Logger.Info("Skipping auth for unauthenticated path", zap.String("path", path))
				return handler(ctx, req)
			}
		}
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		Logger.Error("Missing metadata in request")
		return nil, ErrGRPCMissingMetadata
	}

	// The keys within metadata.MD are normalized to lowercase.
	// See: https://godoc.org/google.golang.org/grpc/metadata#New
	authorization := md["authorization"]
	if len(authorization) < 1 {
		Logger.Error("Missing authorization in metadata", zap.Any("metadata", md))
		return nil, ErrGRPCInvalidToken
	}
	token := strings.TrimPrefix(authorization[0], "Bearer ")
	// Perform the token validation here. For the sake of this example, the code
	// here forgoes any of the usual OAuth2 token validation and instead checks
	// for a token matching an arbitrary string.
	claims, err := LocalJwt.ValidateLocalJwtToken(token)
	if err != nil {
		Logger.Error("failed to validate GRPC token", zap.Error(err))
		return false, ErrGRPCInvalidToken
	}

	// Store the claims in the context
	ctx = context.WithValue(ctx, ContextLocalJwtClaims, claims)

	Logger.Debug("Valid token")
	// Continue execution of handler after ensuring a valid token.
	return handler(ctx, req)
}
