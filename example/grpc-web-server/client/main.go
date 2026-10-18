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
			TLSClientConfig: &tls.Config{RootCAs: certPool, ServerName: "localhost"},
		},
	}

	// WithGRPCWeb makes the connect client use the browser-facing gRPC-Web
	// protocol over plain HTTP/1.1.
	client := genconnect.NewHelloServiceClient(httpClient, "https://127.0.0.1:9001", connect.WithGRPCWeb())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	response, err := client.SayHello(ctx, connect.NewRequest(&gen.HelloRequest{Name: "World"}))
	if err != nil {
		log.Printf("could not greet: %v", err)
		return
	}
	log.Printf("response received: %s", response.Msg.Result)
}
