package main

import (
	"context"
	"log"
	"time"

	"grpc-client/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	conn, err := grpc.NewClient("localhost:9001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		// platform.Logger.Error("failed to connect", zap.Error(err))
	}
	defer conn.Close()

	client := gen.NewHelloServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	response, err := client.SayHello(ctx, &gen.HelloRequest{Name: "World"})
	if err != nil {
		// platform.Logger.Error("failed to say hello", zap.Error(err))
		return
	}
	log.Printf("response received: %s", response.Result)

	// platform.Logger.Info("response received", zap.String("message", response.Message))
}
