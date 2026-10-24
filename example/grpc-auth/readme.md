# gRPC Auth Example

This example runs a Connect server that serves Connect, native gRPC, and
gRPC-Web on one TLS listener. `Login` is public and mints a local JWT;
`SayHello` requires that token.

All settings are programmatic: `server/main.go` is the single source of truth
for the listener, TLS, and local-JWT settings. The example does not read a
`config.yml`.

## Prerequisites

- Go 1.27.2
- `protoc`
- `protoc-gen-go`
- `protoc-gen-connect-go` (the Connect code generator used by `make gen`)
- `openssl` (generates the local TLS certificate and key)
- `grpcurl` (optional, for the manual call below)

WSL:

```bash
sudo apt install -y protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
```

Windows:

```powershell
winget install protobuf
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
```

## Code generation

```bash
make gen
```

This regenerates `server/gen` and `client/gen` from `hello/hello.proto`. The
generated `genconnect` package is what the server registers.

## Certificates

The server and client use the certificate and key produced by:

```bash
make gen-cert
```

The files are written to the example directory and copied into `server/` and
`client/`.

## Build and run

Start the server in one terminal:

```bash
make run-server
```

It listens on `127.0.0.1:9001` with TLS. Then run the bundled client in another
terminal:

```bash
make run-client
```

The client logs in with `user`/`pass` over native gRPC, then calls `SayHello`
with the issued token. A successful run ends with:

```
response received: Hello World
```

## Manual call with grpcurl

Reflection is not enabled, so point grpcurl at the checked-in proto with
`-proto`. Login is public:

```bash
grpcurl -insecure -import-path ./hello -proto ./hello/hello.proto \
  -d '{"Username":"user","Password":"pass"}' \
  127.0.0.1:9001 gen.HelloService/Login
```

Copy the `Token` from the response and use it for the protected method:

```bash
grpcurl -insecure -import-path ./hello -proto ./hello/hello.proto \
  -H "Authorization: Bearer <token>" \
  -d '{"Name":"World"}' \
  127.0.0.1:9001 gen.HelloService/SayHello
```

A missing or invalid token returns `Unauthenticated`.
