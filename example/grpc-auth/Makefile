gen:	
	rm -rf ./server/gen
	rm -rf ./client/gen

	protoc --go_opt=paths=source_relative  --go-grpc_opt=paths=source_relative --go-grpc_out=./hello --go_out=./hello ./hello/hello.proto
	cp -r ./hello/hello ./server/gen
	cp -r ./hello/hello ./client/gen
	rm -rf ./hello/hello

build-server:
	cd ./server && go build -o server

build-client:
	cd ./client && go build -o client

CERT=server.crt
KEY=server.key

gen-cert:
	@echo "Generating certificate and key with SAN for localhost..."
	openssl req -x509 -newkey rsa:4096 -keyout $(KEY) -out $(CERT) -days 365 -nodes \
		-subj "/CN=localhost" \
		-addext "subjectAltName=DNS:localhost"
	cp $(CERT) ./server/
	cp $(KEY) ./server/
	cp $(CERT) ./client/
	cp $(KEY) ./client/

run-server: build-server
	cd ./server && ./server
	

run-client: build-client
	./client/client

clean:
	rm -f ./server/server
	rm -f ./client/client

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  gen           Generate protobuf code"
	@echo "  build-server  Build the server binary"
	@echo "  build-client  Build the client binary"
	@echo "  gen-cert      Generate TLS certificate and key with SAN for localhost"
	@echo "  run-server    Run the server (copies cert/key to ./server)"
	@echo "  run-client    Run the client (copies cert/key to ./client)"
	@echo "  clean         Remove server and client binaries"