package platform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const panicProcedure = "/test.PanicService/Panic"
const panicStreamProcedure = "/test.PanicService/PanicStream"

type panicService struct{}

func (p *panicService) Register(mux *http.ServeMux, options ...connect.HandlerOption) {
	mux.Handle(panicProcedure, connect.NewUnaryHandler(
		panicProcedure,
		func(context.Context, *connect.Request[wrapperspb.StringValue]) (*connect.Response[wrapperspb.StringValue], error) {
			panic("unary boom")
		},
		options...,
	))

	mux.Handle(panicStreamProcedure, connect.NewServerStreamHandler(
		panicStreamProcedure,
		func(context.Context, *connect.Request[wrapperspb.StringValue], *connect.ServerStream[wrapperspb.StringValue]) error {
			panic("stream boom")
		},
		options...,
	))
}

func TestConnectRecoveryInterceptorUnary(t *testing.T) {
	withTestDB(t)
	handler, err := NewConnectHandler(ConnectServerOptions{
		Services:              []ConnectService{&panicService{}},
		DisableAuthFromConfig: true,
	})
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+panicProcedure)

	_, callErr := client.CallUnary(context.Background(), connect.NewRequest(wrapperspb.String("x")))
	if connect.CodeOf(callErr) != connect.CodeInternal {
		t.Fatalf("expected CodeInternal, got %v (%v)", connect.CodeOf(callErr), callErr)
	}

	var connectErr *connect.Error
	if !errors.As(callErr, &connectErr) {
		t.Fatalf("expected a connect error, got %T", callErr)
	}
	if connectErr.Meta().Get(RequestIDHeader) == "" {
		t.Fatal("expected the request id in the error metadata")
	}
}

func TestConnectRecoveryInterceptorStreaming(t *testing.T) {
	withTestDB(t)
	handler, err := NewConnectHandler(ConnectServerOptions{
		Services:              []ConnectService{&panicService{}},
		DisableAuthFromConfig: true,
	})
	if err != nil {
		t.Fatalf("NewConnectHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	client := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		server.Client(), server.URL+panicStreamProcedure)

	stream, err := client.CallServerStream(context.Background(), connect.NewRequest(wrapperspb.String("x")))
	if err != nil {
		t.Fatalf("CallServerStream failed: %v", err)
	}
	defer stream.Close()

	if stream.Receive() {
		t.Fatal("expected no messages from a panicking stream")
	}
	if connect.CodeOf(stream.Err()) != connect.CodeInternal {
		t.Fatalf("expected CodeInternal, got %v (%v)", connect.CodeOf(stream.Err()), stream.Err())
	}
}

func TestConnectRequestIDHeaderOnSuccessAndFailure(t *testing.T) {
	withTestDB(t)

	handler := newEchoHandler(t, ConnectServerOptions{DisableAuthFromConfig: true})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := echoClient(server, server.Client())
	request := connect.NewRequest(wrapperspb.String("x"))
	request.Header().Set(RequestIDHeader, "caller-supplied")
	response, err := client.CallUnary(context.Background(), request)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := response.Header().Get(RequestIDHeader); got != "caller-supplied" {
		t.Fatalf("expected the caller request id on the response, got %q", got)
	}

	// Generated ids also reach the response.
	response, err = client.CallUnary(context.Background(), connect.NewRequest(wrapperspb.String("y")))
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if response.Header().Get(RequestIDHeader) == "" {
		t.Fatal("expected a generated request id on the response")
	}
}

func TestConnectAuthFailureCarriesRequestID(t *testing.T) {
	configureConnectLocalJWT(t)

	server := httptest.NewServer(newEchoHandler(t, ConnectServerOptions{}))
	defer server.Close()

	client := echoClient(server, server.Client())
	_, err := callEcho(t, client, "")
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", connect.CodeOf(err))
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected a connect error, got %T", err)
	}
	if connectErr.Meta().Get(RequestIDHeader) == "" {
		t.Fatal("expected the request id on auth failures")
	}
}

func TestRPCTransportClassification(t *testing.T) {
	testCases := []struct {
		contentType string
		want        string
	}{
		{contentType: "application/grpc", want: "grpc"},
		{contentType: "application/grpc+proto", want: "grpc"},
		{contentType: "application/grpc-web+proto", want: "grpc-web"},
		{contentType: "application/grpc-web-text; charset=utf-8", want: "grpc-web"},
		{contentType: "application/proto", want: "connect"},
		{contentType: "application/json", want: "connect"},
	}

	for _, testCase := range testCases {
		header := http.Header{"Content-Type": []string{testCase.contentType}}
		if got := rpcTransport(header); got != testCase.want {
			t.Fatalf("for %q expected %q, got %q", testCase.contentType, testCase.want, got)
		}
	}
}
