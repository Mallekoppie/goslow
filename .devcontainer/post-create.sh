#!/usr/bin/env bash
# Runs once after the container is created (see devcontainer.json).
set -euo pipefail

workspace_dir="$(pwd)"
echo "==> Preparing Go caches in ${workspace_dir}"

mod_cache="$(go env GOMODCACHE)"
build_cache="$(go env GOCACHE)"
mkdir -p "${mod_cache}" "${build_cache}"

# Named volumes are normally populated with the image's ownership, but some
# container engines create them as root. Fix that up rather than failing.
if [ ! -w "${mod_cache}" ] || [ ! -w "${build_cache}" ]; then
  echo "==> Fixing cache ownership"
  sudo chown -R "$(id -u):$(id -g)" "${mod_cache}" "${build_cache}"
fi

echo "==> Downloading modules for every module in the repo"
find . -name go.mod -not -path './.git/*' -print0 |
  while IFS= read -r -d '' modfile; do
    dir="$(dirname "${modfile}")"
    echo "    - ${dir}"
    (cd "${dir}" && go mod download) ||
      echo "    ! could not download modules in ${dir} (network or go.sum issue)"
  done

echo "==> Toolchain"
go version
protoc --version
gopls version | head -n 1
staticcheck -version
if command -v protoc-gen-connect-go >/dev/null 2>&1; then
  protoc-gen-connect-go --version 2>/dev/null || echo "    protoc-gen-connect-go installed"
else
  echo "    ! protoc-gen-connect-go missing: go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest"
fi

cat <<'EOF'

==> Ready. Useful commands:
    go test -race ./...                    # root module
    go test -race -tags integration ./...  # tests needing a live IdP/Vault
    go run ./example/boltdb                # BoltDB example
    make -C example/grpc-auth gen          # regenerate protobuf + Connect stubs

Podman note: if the workspace ends up owned by root inside the container,
add "--userns=keep-id" to "runArgs" and set "updateRemoteUserUID": false.
See .devcontainer/README.md.
EOF
