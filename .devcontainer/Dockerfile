# syntax=docker/dockerfile:1

# goslow is a multi-module Go workspace: the root module targets Go 1.25,
# while example modules pin older versions. A Go 1.25 toolchain builds those
# fine; note this image sets GOTOOLCHAIN=local, so nothing newer than 1.25 can
# be pulled in automatically.
ARG GO_VERSION=1.25
FROM mcr.microsoft.com/devcontainers/go:${GO_VERSION}-bookworm

ENV GOPATH=/go
ENV PATH=$GOPATH/bin:$PATH

# protoc is needed by example/grpc-auth and example/grpc-web-server Makefiles.
RUN apt-get update \
    && export DEBIAN_FRONTEND=noninteractive \
    && apt-get install -y --no-install-recommends \
        protobuf-compiler \
        make \
        jq \
        bash-completion \
    && apt-get clean -y \
    && rm -rf /var/lib/apt/lists/*

# Pre-create cache directories so the named volumes in devcontainer.json
# inherit the vscode user's ownership instead of root's.
RUN mkdir -p /go/pkg/mod /home/vscode/.cache/go-build \
    && chown -R vscode:vscode /go /home/vscode/.cache

USER vscode

# Tools used by the repo's test/CI plan and examples.
#
# Versions are pinned to releases whose `go` directive is <= the image's Go
# version. The image sets GOTOOLCHAIN=local, so `@latest` breaks the build as
# soon as a tool starts requiring a newer Go (e.g. gopls v0.24 needs Go 1.27).
# Bump these together with GO_VERSION above.
ARG GOPLS_VERSION=v0.21.1
ARG STATICCHECK_VERSION=v0.7.0
ARG DELVE_VERSION=v1.27.2
ARG PROTOC_GEN_GO_VERSION=v1.36.12
ARG PROTOC_GEN_GO_GRPC_VERSION=v1.6.2

RUN go install golang.org/x/tools/gopls@${GOPLS_VERSION} \
    && go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION} \
    && go install github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION} \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION} \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}
