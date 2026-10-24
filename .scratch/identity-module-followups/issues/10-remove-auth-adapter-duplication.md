# 10: Remove the remaining auth-adapter duplication

**What to build:** The HTTP and RPC adapters classify authorization failures once instead of maintaining two matching switches. The retry delay advertised for an unavailable identity provider is defined once, and the ID-token verifier is built from an OAuth settings value rather than a fabricated partial configuration. Behaviour and transport mappings are unchanged.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] One classification produces a transport-neutral outcome; each adapter maps that outcome to its status or code.
- [x] The retry delay is defined in one place and used by every adapter and example that advertises it.
- [x] No partial configuration is constructed solely to carry OAuth settings to the verifier.
- [x] Existing tests pass without changing the observable 401/403/503 and unauthenticated/permission-denied/unavailable mappings.

## Comments

Implemented on `ticket/10-remove-auth-adapter-duplication` at source commit `e636272` (branch tip `7a3667c`). Merged into `identity-module-followups` with `git merge --no-ff` as merge commit `a38e02e`; no conflicts.

Verification from `/workspaces/goslow`: `go build ./...`, `go vet ./...`, `gofmt -l .` (no output), `go test -count=1 ./...`, `go test -count=1 -race ./...`, and `make build-examples` all passed.
