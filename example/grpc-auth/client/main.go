package main

import (
	"context"
	"time"

	"grpc-client/gen"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
)

func main() {
	conn, err := grpc.NewClient("localhost:9001", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithInsecure())
	if err != nil {
		platform.Logger.Error("failed to connect", zap.Error(err))
	}
	defer conn.Close()

	client := gen.NewHelloServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	loginResponse, err := client.Login(ctx, &gen.LoginRequest{Username: "user", Password: "pass"})
	if err != nil {
		platform.Logger.Error("failed to login", zap.Error(err))
		return
	}
	if loginResponse.Success {
		platform.Logger.Info("Login successful", zap.String("token", loginResponse.Token))
	} else {
		platform.Logger.Error("Login failed", zap.String("message", loginResponse.Message))
		return
	}

	callOption := grpc.PerRPCCredentials(oauth.TokenSource{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: loginResponse.Token})})
	response, err := client.SayHello(ctx, &gen.HelloRequest{Name: "World"}, callOption)
	if err != nil {
		platform.Logger.Error("failed to say hello", zap.Error(err))
		return
	}
	platform.Logger.Info("response received", zap.String("message", response.Result))

	// platform.Logger.Info("response received", zap.String("message", response.Message))
}
