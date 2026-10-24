# 04: Make the HTTP examples honour the identity contract

**What to build:** The HTTP examples demonstrate the glossary's mapping for callers that reach a handler. A verified caller lacking the required role receives 403, not 401, and receives a documented success response when it holds the role. A protected handler that somehow runs without an identity fails closed instead of reporting success.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] The web-server OAuth handler returns 403 for a verified caller lacking the required role.
- [x] The same handler returns a success response describing the caller when the role is present, rather than only logging.
- [x] The local-JWT example's protected query handler fails closed when no identity is present; it never returns a success message.
- [x] Both examples still build and run as documented.

## Comments

Resolved. Source commit `8556810` (Make HTTP examples honour the identity contract) merged into `identity-module-followups` as merge commit `b331ec28c4cfdf11c6abd561b6173649b6ca3d14`. No merge conflicts.

Verification performed from `/workspaces/goslow`:

- `go build ./...` — pass
- `go vet ./...` — pass
- `gofmt -l .` — no files reported
- `go test ./...` — pass
- `go test -race ./...` — pass
- `make build-examples` — pass (all seven example modules built)
- `go test ./...` in `example/http-web-server` — pass
- `go test -race ./...` in `example/http-web-server` — pass
- `go test ./...` in `example/localJwt` — pass
- `go test -race ./...` in `example/localJwt` — pass
