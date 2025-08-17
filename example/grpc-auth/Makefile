gen:	
	rm -rf ./server/gen
	rm -rf ./client/gen

	protoc --go_opt=paths=source_relative  --go-grpc_opt=paths=source_relative --go-grpc_out=./hello --go_out=./hello ./hello/hello.proto
	cp -r ./hello/hello ./server/gen
	cp -r ./hello/hello ./client/gen
	rm -rf ./hello/hello