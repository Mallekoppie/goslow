module grpc-client

go 1.24.5

replace github.com/Mallekoppie/goslow => ../../../

require (
	google.golang.org/grpc v1.74.2
	google.golang.org/protobuf v1.36.7

)

require (
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
)
