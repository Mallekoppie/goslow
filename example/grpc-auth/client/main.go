package main

import (
	"context"
	"crypto/x509"
	"log"
	"os"
	"time"

	"grpc-client/gen"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

func main() {
	certBytes, err := os.ReadFile("server.crt")
	if err != nil {
		log.Fatalf("failed to read server certificate: %v", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(certBytes) {
		log.Fatalf("failed to append server certificate to pool")
	}
	creds := credentials.NewClientTLSFromCert(certPool, "localhost")

	conn, err := grpc.NewClient("127.0.0.1:9001", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
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
