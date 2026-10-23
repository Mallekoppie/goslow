# syntax=docker/dockerfile:1

# goslow is a multi-module Go workspace: the root module targets Go 1.27,
# while example modules pin matching versions. This image sets GOTOOLCHAIN=local,
# so nothing newer than 1.27 can be pulled in automatically.
ARG GO_VERSION=1.27
FROM mcr.microsoft.com/devcontainers/go:${GO_VERSION}-bookworm

ENV GOPATH=/go
# The node devcontainer feature installs Node/npm - and the claude CLI it
# installs globally - under nvm. Add those locations so non-login shells see
# them; the profile.d snippet at the end of this file covers login shells.
ENV PATH=/usr/local/share/nvm/current/bin:/home/vscode/.local/bin:$GOPATH/bin:$PATH

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
ARG GOPLS_VERSION=v0.24.0
ARG STATICCHECK_VERSION=v0.8.2
ARG DELVE_VERSION=v1.27.2
ARG PROTOC_GEN_GO_VERSION=v1.36.12
ARG PROTOC_GEN_CONNECT_GO_VERSION=v1.21.1

RUN go install golang.org/x/tools/gopls@${GOPLS_VERSION} \
    && go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION} \
    && go install github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION} \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION} \
    && go install connectrpc.com/connect/cmd/protoc-gen-connect-go@${PROTOC_GEN_CONNECT_GO_VERSION}

# A login shell (`bash -lc`, which VS Code dev containers and postCreateCommand
# use) resets PATH from /etc/profile and would otherwise drop the nvm,
# user-local and Go bin directories. Register them in profile.d so every shell
# - interactive, login or remote - sees gopls, dlv, claude and friends.
USER root
RUN printf '%s\n' \
        'export PATH="/usr/local/share/nvm/current/bin:/go/bin:/home/vscode/.local/bin:$PATH"' \
        > /etc/profile.d/00-devcontainer-path.sh \
    && chmod 0644 /etc/profile.d/00-devcontainer-path.sh

# Pre-create the mount point for Claude Code's config volume (see
# devcontainer.json). A fresh named volume inherits the ownership of the image
# path, so this is what makes it writable by the non-root vscode user.
RUN mkdir -p /home/vscode/.claude \
    && chown vscode:vscode /home/vscode/.claude

USER vscode
