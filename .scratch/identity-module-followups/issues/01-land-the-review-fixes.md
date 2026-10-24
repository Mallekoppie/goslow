# 01: Land the review fixes

**What to build:** The branch carries the four review corrections as one committed change: the HTTP OAuth example points at an issuer base rather than a discovery document, the README no longer advertises a verifier-injection path that no public server constructor exposes, the README route example declares an explicit auth strategy on every route, and the configuration-defaults table lists the local-JWT roles claim. The repository baseline stays green.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] The four corrections are present and committed with a message explaining what was wrong.
- [x] The documented route example matches the mandatory explicit-strategy rule stated in the same document.
- [x] The documented defaults table lists the default roles claim for both OAuth and local JWT.
- [x] `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`, and `go test -race ./...` pass.
- [x] No stale user-facing reference to the removed verifier-injection advice remains.

## Comments

Completed in `161a365`. The HTTP OAuth example now uses an issuer base URL;
the README's verifier-injection claim, route example, and defaults table are
corrected. Verified with `go vet ./...`, `gofmt -l .`, `go test ./...`,
`go test -race ./...`, `make build-examples`, and `go mod tidy -diff`.
