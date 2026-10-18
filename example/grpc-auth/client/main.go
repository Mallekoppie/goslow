package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"net/http"
	"os"
	"time"

	"grpc-client/gen"
	"grpc-client/gen/genconnect"

	"connectrpc.com/connect"
	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
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

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{RootCAs: certPool, ServerName: "localhost"},
			ForceAttemptHTTP2: true, // required for the gRPC protocol
		},
	}

	// WithGRPC makes the connect client speak the native gRPC protocol, proving
	// the server is gRPC compatible without importing google.golang.org/grpc.
	client := genconnect.NewHelloServiceClient(httpClient, "https://127.0.0.1:9001", connect.WithGRPC())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loginResponse, err := client.Login(ctx, connect.NewRequest(&gen.LoginRequest{Username: "user", Password: "pass"}))
	if err != nil {
		platform.Log.Error("failed to login", zap.Error(err))
		return
	}
	if !loginResponse.Msg.Success {
		platform.Log.Error("Login failed", zap.String("message", loginResponse.Msg.Message))
		return
	}
	platform.Log.Info("Login successful", zap.String("token", platform.RedactToken(loginResponse.Msg.Token)))

	helloRequest := connect.NewRequest(&gen.HelloRequest{Name: "World"})
	helloRequest.Header().Set("Authorization", "Bearer "+loginResponse.Msg.Token)

	response, err := client.SayHello(ctx, helloRequest)
	if err != nil {
		platform.Log.Error("failed to say hello", zap.Error(err))
		return
	}
	platform.Log.Info("response received", zap.String("message", response.Msg.Result))
}
