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
if command -v claude >/dev/null 2>&1; then
  echo "claude: $(claude --version 2>/dev/null | head -n 1)"
else
  echo "    ! claude missing - rebuild the container to install the devcontainer feature"
fi
if command -v protoc-gen-connect-go >/dev/null 2>&1; then
  protoc-gen-connect-go --version 2>/dev/null || echo "    protoc-gen-connect-go installed"
else
  echo "    ! protoc-gen-connect-go missing: go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.21.1"
fi

# Install the Matt Pocock skills plugin into the persistent config volume.
# Best-effort: an offline first create must not block opening the repo, and the
# command is repeated in the summary below for a manual retry.
if command -v claude >/dev/null 2>&1; then
  echo "==> Claude Code skills plugin"
  if claude plugin install mattpocock-skills \
       --marketplace anthropics/claude-plugins-official \
       --scope user -y </dev/null >/dev/null 2>&1; then
    echo "    mattpocock-skills ready"
  else
    echo "    ! could not install mattpocock-skills (offline?) - see the command below"
  fi
fi

cat <<'EOF'

==> Ready. Useful commands:
    claude                                 # first run signs in if no token is forwarded
    go test -race ./...                    # root module
    go test -race -tags integration ./...  # tests needing a live IdP/Vault
    go run ./example/boltdb                # BoltDB example
    make -C example/grpc-auth gen          # regenerate protobuf + Connect stubs

Claude Code config (login, settings, plugins) persists in the named volume
mounted at /home/vscode/.claude, so it survives rebuilds. The mattpocock-skills
plugin is enabled by .claude/settings.json and installed by this script. If the
install was skipped (offline), run:
    claude plugin install mattpocock-skills \
        --marketplace anthropics/claude-plugins-official
Then run the setup skill in Claude to tailor it to this repo:
    /mattpocock-skills:setup-matt-pocock-skills

Podman note: runArgs uses --userns=keep-id so the container uid matches the
host uid that owns the workspace. See .devcontainer/README.md.
EOF
