package main

import (
	"context"
	"log"
	"time"

	"grpc-client/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	creds, err := credentials.NewClientTLSFromFile("server.crt", "localhost")
	if err != nil {
		log.Fatalf("failed to create TLS credentials: %v", err)
	}

	conn, err := grpc.NewClient("127.0.0.1:9001", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	client := gen.NewHelloServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	state := conn.GetState()
	log.Printf("gRPC connection state: %v", state)

	response, err := client.SayHello(ctx, &gen.HelloRequest{Name: "World"})
	if err != nil {
		log.Printf("could not greet: %v", err)
		return
	}
	log.Printf("response received: %s", response.Result)

	// platform.Logger.Info("response received", zap.String("message", response.Message))
}
