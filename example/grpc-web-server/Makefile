CERT=server.crt
KEY=server.key

gen:
	rm -rf ./server/gen ./client/gen ./tmpgen ./tmpgen-client ./hello/hello

	protoc --go_opt=paths=source_relative --go_out=./hello ./hello/hello.proto

	mkdir -p ./tmpgen ./tmpgen-client
	protoc --connect-go_opt=paths=source_relative \
		--connect-go_opt=Mhello/hello.proto=grpc-server/gen \
		--connect-go_out=./tmpgen ./hello/hello.proto
	protoc --connect-go_opt=paths=source_relative \
		--connect-go_opt=Mhello/hello.proto=grpc-client/gen \
		--connect-go_out=./tmpgen-client ./hello/hello.proto

	mkdir -p ./server/gen ./client/gen
	cp ./hello/hello/hello.pb.go ./server/gen/
	cp -r ./tmpgen/hello/genconnect ./server/gen/
	cp ./hello/hello/hello.pb.go ./client/gen/
	cp -r ./tmpgen-client/hello/genconnect ./client/gen/

	rm -rf ./hello/hello ./tmpgen ./tmpgen-client

build-server:
	cd ./server && go build -o server

build-client:
	cd ./client && go build -o client

run-server: build-server
	cd ./server && ./server

run-client: build-client
	cd ./client && ./client

clean:
	rm -f ./server/server
	rm -f ./client/client

gen-cert:
	@echo "Generating certificate and key with SAN for localhost..."
	openssl req -x509 -newkey rsa:4096 -keyout $(KEY) -out $(CERT) -days 365 -nodes \
		-subj "/CN=localhost" \
		-addext "subjectAltName=DNS:localhost"
	cp $(CERT) ./server/
	cp $(KEY) ./server/
	cp $(CERT) ./client/
	cp $(KEY) ./client/

