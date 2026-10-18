package platform

import (
	"context"
	"embed"
	"io/fs"
	"net"
	"net/http"
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

func StartGrpcServerWithWeb(services []GRPCService, webDirectoryName string, webAssets *embed.FS) {
	InitializeLogger()

	config, err := GetPlatformConfiguration()
	if err != nil {
		Log.Error("Error reading platform configuration", zap.Error(err))
		panic(err)
	}

	conf := config.Grpc.Server
	Log.Info("Starting new GRPC server", zap.String("ListeningAddress", conf.ListeningAddress))
	lis, err := net.Listen("tcp", conf.ListeningAddress)
	if err != nil {
		Log.Error("failed to listen", zap.Error(err))
		return
	}

	var uiHandler http.Handler
	webEnabled := false

	if len(webDirectoryName) > 0 {
		webEnabled = true
		uiFs, err := fs.Sub(webAssets, webDirectoryName)
		if err != nil {
			Log.Error("failed to create sub filesystem", zap.Error(err))
		}
		uiHandler = http.FileServer(http.FS(uiFs))
	}

	serverOptions := []grpc.ServerOption{}

	if conf.TLSEnabled {
		creds, err := credentials.NewServerTLSFromFile(conf.TLSCertFileName, conf.TLSKeyFileName)
		if err != nil {
			Log.Error("failed to load TLS credentials for GRPC server", zap.Error(err))
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

	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route gRPC requests to grpcServer
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}
		// Route everything else to UI
		uiHandler.ServeHTTP(w, r)
	})

	if webEnabled {
		if err := http.ServeTLS(lis, mux, conf.TLSCertFileName, conf.TLSKeyFileName); err != nil {
			Log.Error("failed to serve web content", zap.Error(err))
		}
	} else {
		if err := grpcServer.Serve(lis); err != nil {
			Log.Error("failed to serve gRPC", zap.Error(err))
		}
	}
}

func StartGrpcServer(services []GRPCService) {
	StartGrpcServerWithWeb(services, "", nil)
}

func grpcAuthInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	conf, confErr := GetPlatformConfiguration()
	if confErr != nil || conf == nil {
		Log.Error("Unable to load platform configuration for gRPC auth", zap.Error(confErr))
		return nil, status.Errorf(codes.Internal, "platform configuration unavailable")
	}

	for _, path := range conf.Grpc.Server.UnAuthenticatedPaths {
		if path == info.FullMethod {
			Log.Info("Skipping auth for unauthenticated path", zap.String("path", path))
			return handler(ctx, req)
		}
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		Log.Error("Missing metadata in request")
		return nil, ErrGRPCMissingMetadata
	}

	// The keys within metadata.MD are normalized to lowercase.
	// See: https://godoc.org/google.golang.org/grpc/metadata#New
	authorization := md["authorization"]
	if len(authorization) < 1 {
		Log.Error("Missing authorization in metadata", zap.Any("metadata", md))
		return nil, ErrGRPCInvalidToken
	}
	token, ok := bearerToken(authorization[0])
	if !ok {
		Log.Error("authorization metadata is not a bearer token")
		return nil, ErrGRPCInvalidToken
	}

	claims, err := LocalJwt.ValidateLocalJwtToken(token)
	if err != nil {
		Log.Error("failed to validate GRPC token", zap.Error(err))
		return nil, ErrGRPCInvalidToken
	}

	// Store the claims in the context
	ctx = context.WithValue(ctx, ContextLocalJwtClaims, claims)

	Log.Debug("Valid token")
	// Continue execution of handler after ensuring a valid token.
	return handler(ctx, req)
}
