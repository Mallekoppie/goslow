package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"grpc-server/gen"
	"grpc-server/gen/genconnect"

	"connectrpc.com/connect"
	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
)

// Server implements the generated Connect handler interface. Connect serves the
// same service over Connect, gRPC, and gRPC-Web, so no grpc-go server is needed.
type Server struct{}

func (s *Server) Login(ctx context.Context, request *connect.Request[gen.LoginRequest]) (*connect.Response[gen.LoginResponse], error) {
	platform.Log.Info("Received Login request", zap.String("username", request.Msg.Username))

	if request.Msg.Username == "user" && request.Msg.Password == "pass" {
		token, err := platform.LocalJwt.NewLocalJwtToken(map[string]interface{}{
			"username": request.Msg.Username,
		})
		if err != nil {
			platform.Log.Error("failed to create token", zap.Error(err))
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create token"))
		}

		platform.Log.Info("Login successful", zap.String("user", request.Msg.Username))
		return connect.NewResponse(&gen.LoginResponse{
			Token:   token,
			Success: true,
			Message: "login successful",
		}), nil
	}

	return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
}

func (s *Server) SayHello(ctx context.Context, request *connect.Request[gen.HelloRequest]) (*connect.Response[gen.HelloResponse], error) {
	platform.Log.Info("Received SayHello request", zap.String("name", request.Msg.Name))
	return connect.NewResponse(&gen.HelloResponse{Result: "Hello " + request.Msg.Name}), nil
}

// Register is the platform.ConnectService hook: it mounts the generated Connect
// handler, which also serves gRPC and gRPC-Web for this service.
func (s *Server) Register(mux *http.ServeMux, options ...connect.HandlerOption) {
	prefix, handler := genconnect.NewHelloServiceHandler(s, options...)
	mux.Handle(prefix, handler)
}

func main() {
	config := platform.Config{}
	config.Component.ComponentName = "grpc-auth"
	config.Grpc.Server.ListeningAddress = "127.0.0.1:9001"
	config.Grpc.Server.TLSCertFileName = "server.crt"
	config.Grpc.Server.TLSKeyFileName = "server.key"
	config.Grpc.Server.TLSEnabled = true
	config.Grpc.Server.Auth = platform.AuthLocalJWT
	config.Auth.Server.UnauthenticatedPaths = []string{"/gen.HelloService/Login"}
	config.Auth.Server.LocalJwt.Enabled = true
	config.Auth.Server.LocalJwt.JwtSigningKey = "blablablacksheep"
	config.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	config.Auth.Server.LocalJwt.JwtExpiration = 60

	platform.SetPlatformConfiguration(config)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	services := []platform.ConnectService{
		&Server{},
	}

	if err := platform.StartConnectServerWithContext(ctx, platform.ConnectServerOptions{Services: services}); err != nil {
		platform.Log.Error("Connect server stopped", zap.Error(err))
	}
}
