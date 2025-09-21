# Install
## WSL
```
sudo apt install -y protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```


## Win
```
winget install protobuf
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```



# Testing

grpcurl -insecure -import-path ./server/gen -import-path ./hello -proto ./hello/hello.proto localhost:9001 list

returns
gen.HelloService


grpcurl -insecure -import-path ./server/gen -import-path ./hello -proto ./hello/hello.proto localhost:9001 gen.HelloService list


# Example: Call SayHello using grpcurl

grpcurl -insecure -import-path ./server/gen -import-path ./hello -proto ./hello/hello.proto \
	-d '{"Name":"World"}' \
	localhost:9001 gen.HelloService.SayHello