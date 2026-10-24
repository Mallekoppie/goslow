# gRPC-Web Example

This example serves a Connect service and an embedded web page on one TLS
listener. The generated Connect handler answers Connect, native gRPC, and
gRPC-Web requests; the bundled client uses the gRPC-Web protocol.

## Prerequisites

- Go 1.27.2
- `protoc`
- `protoc-gen-go`
- `protoc-gen-connect-go` (the Connect code generator used by `make gen`)
- `openssl` (generates the local TLS certificate and key)
- `grpcurl` (optional, for the manual checks below)

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

This regenerates `server/gen` and `client/gen` from `hello/hello.proto`.

## Certificates

The server and client use the certificate and key produced by:

```bash
make gen-cert
```

## Build and run

Start the server in one terminal:

```bash
make run-server
```

It listens on `127.0.0.1:9001` with TLS and serves the embedded `server/ui`
assets alongside the RPCs. Then run the bundled client in another terminal:

```bash
make run-client
```

A successful run prints:

```
response received: Hello World
```

## Manual checks with grpcurl

The server does not enable reflection, so grpcurl reads the checked-in proto
with `-proto`; do not pass `-use-reflection`.

List the services:

```bash
grpcurl -insecure -import-path ./hello -proto ./hello/hello.proto localhost:9001 list
```

This returns:

```
gen.HelloService
```

List the methods:

```bash
grpcurl -insecure -import-path ./hello -proto ./hello/hello.proto localhost:9001 list gen.HelloService
```

Call `SayHello`:

```bash
grpcurl -insecure -import-path ./hello -proto ./hello/hello.proto \
  -d '{"Name":"World"}' \
  localhost:9001 gen.HelloService.SayHello
```

This returns:

```json
{
  "Result": "Hello World"
}
```
