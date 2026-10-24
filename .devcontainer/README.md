# goslow devcontainer

Go 1.27 development container for the goslow platform and its example modules.

## What is included

| Item | Why |
| --- | --- |
| `mcr.microsoft.com/devcontainers/go:1.27-bookworm` | Matches the root `go.mod`. `GOTOOLCHAIN=local` keeps builds on the image toolchain. |
| `protobuf-compiler`, `make`, `jq` | `example/grpc-auth` and `example/grpc-web-server` Makefiles call `protoc`. |
| `gopls`, `staticcheck`, `dlv`, `protoc-gen-go`, `protoc-gen-connect-go` | Editor tooling plus Connect code generation for the gRPC-compatible examples. |
| Named volumes for `/go/pkg/mod` and the build cache | Keeps `-race` builds fast and off the workspace bind mount. |
| Node.js | LTS (devcontainer feature); required by Claude Code. |
| Claude Code | latest (devcontainer feature); `claude` CLI + VS Code extension. |
| GitHub CLI (`gh`) | Issue tracking and pull requests; auth persists in the `goslow-gh-config` volume. |
| `post-create.sh` | Downloads modules for the root module and every example module, reports the toolchain, installs the skills plugin. |

## Opening the project

1. VS Code: **Dev Containers: Reopen in Container**.
2. The container user is `vscode`, mapped through rootless Podman's keep-id so
   it matches the host UID that owns the workspace.

## Podman notes (Bazzite / Fedora Atomic)

- `runArgs` contains `--userns=keep-id` so the container's uid 1000 matches the
  host uid (1000) that owns the bind-mounted workspace and the named volumes.
- `--security-opt label=disable` is required because SELinux cannot label the
  bind-mounted workspace.
- VS Code needs to be pointed at Podman once (user settings, not repo settings):
  `"dev.containers.dockerPath": "podman"`.
- `--userns=keep-id` is a Podman flag; if you open this repo with Docker instead,
  remove it from `runArgs`.

## Claude Code

Claude Code is installed by the official dev container feature:

```json
"features": {
  "ghcr.io/devcontainers/features/node:1": {},
  "ghcr.io/anthropics/devcontainer-features/claude-code:1.0": {}
}
```

The Go base image ships nvm but does not install a Node version, so the Node
feature comes first — the Claude feature declares an `installsAfter` dependency
on it. The feature then installs the `claude` CLI globally and adds the Claude
Code VS Code extension, so the tool works both from the integrated terminal and
from the extension panel.

### Host environment

`remoteEnv` copies these variables from the host into terminals, tasks and
extensions in the container:

* `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`
* `ANTHROPIC_BASE_URL`, `ANTHROPIC_CUSTOM_HEADERS`, `ANTHROPIC_MODEL`
* `ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU,FABLE}_MODEL`

Export them on the host before opening or rebuilding the container, e.g.
`export ANTHROPIC_API_KEY=...`. Unset variables arrive as empty strings and are
ignored by Claude Code. To forward another variable, add
`"NAME": "${localEnv:NAME}"` to `remoteEnv` and rebuild.

### Authentication and persistence

If no API key or token is forwarded, run `claude` in the container and complete
the browser sign-in. If the callback does not reach the container, copy the code
shown in the browser back into the terminal. Authentication, settings, plugins
and session history live under `/home/vscode/.claude`, which is the named volume
`claude-code-config-<id>`, so they survive **Dev Containers: Rebuild Container**.
`CLAUDE_CONFIG_DIR` points at that volume so `~/.claude.json` is persisted too.

### Matt Pocock skills

`.claude/settings.json` enables `mattpocock-skills` from the official plugin
marketplace. `post-create.sh` installs it once into the persistent volume:

```bash
claude plugin install mattpocock-skills \
  --marketplace anthropics/claude-plugins-official
```

If the install was skipped (e.g. offline during create), run it manually. Then
run the setup skill once per repository so the skills know where issues, specs
and domain docs live:

```text
/mattpocock-skills:setup-matt-pocock-skills
```

## GitHub CLI

`gh` is installed from GitHub's apt repository by the `Dockerfile`. Authenticate
once after the container is created:

```bash
gh auth login
```

Authentication is stored under `/home/vscode/.config/gh`, which is the named
volume `goslow-gh-config`, so it survives **Dev Containers: Rebuild Container**.
The issue tracker, pull requests, and the skills' GitHub workflows use this CLI;
see `docs/agents/issue-tracker.md`. Work that was tracked in local markdown
before this switch is not recreated as GitHub issues.

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
