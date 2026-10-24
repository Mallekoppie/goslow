# 03: Retire the test-only local-JWT verifier

**What to build:** Local JWT verification exists in exactly one place, the authorizer. The organizer-level validation method that reads global configuration and is exercised only by tests is deleted, and its tests move to the authorizer with a settings snapshot and a controlled signing key. Token minting remains available for login flows.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The organizer-level local-JWT validation method is deleted and no production or example code references it.
- [ ] Tests formerly covering it assert the same outcomes through the authorizer: valid token, expired token, foreign signature, malformed token, and rejected signing method.
- [ ] Local JWT minting used by login handlers is unchanged.
- [ ] `go build ./...`, `go vet ./...`, `gofmt -l .`, and the full test suite pass.
