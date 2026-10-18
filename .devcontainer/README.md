# goslow devcontainer

Go 1.25 development container for the goslow platform and its example modules.

## What is included

| Item | Why |
| --- | --- |
| `mcr.microsoft.com/devcontainers/go:1.25-bookworm` | Matches the root `go.mod`. `GOTOOLCHAIN=auto` covers older example modules. |
| `protobuf-compiler`, `make`, `jq` | `example/grpc-auth` and `example/grpc-web-server` Makefiles call `protoc`. |
| `gopls`, `staticcheck`, `dlv`, `protoc-gen-go`, `protoc-gen-connect-go` | Editor tooling plus Connect code generation for the gRPC-compatible examples. |
| Named volumes for `/go/pkg/mod` and the build cache | Keeps `-race` builds fast and off the workspace bind mount. |
| `post-create.sh` | Downloads modules for the root module and every example module. |

## Opening the project

1. VS Code: **Dev Containers: Reopen in Container**.
2. The container user is `vscode`; `updateRemoteUserUID` aligns it with your host UID on Linux.

## Podman notes (Bazzite / Fedora Atomic)

- `runArgs` already contains `--security-opt label=disable` because SELinux
  cannot label the bind-mounted workspace.
- VS Code needs to be pointed at Podman once (user settings, not repo settings):
  `"dev.containers.dockerPath": "podman"`.
- If files in `/workspaces/goslow` appear owned by `root` inside the container,
  switch to keep-id mapping:
  - add `"--userns=keep-id"` to `runArgs` in `devcontainer.json`
  - set `"updateRemoteUserUID": false`

## Ports

| Port | Used by |
| --- | --- |
| `9001` | gRPC examples |
| `9111` | `platform/config.yml` HTTP listener |
| `10000` | platform HTTP default |

## Tests

The root module is tested with `go test -race ./...`. Tests that need a live
identity provider or Vault are gated behind the `integration` build tag:

```bash
go test -race ./...
go test -race -tags integration ./platform/...
```

Note that several example directories are separate Go modules. Run their
commands from inside the module directory (or use `make` where provided).
