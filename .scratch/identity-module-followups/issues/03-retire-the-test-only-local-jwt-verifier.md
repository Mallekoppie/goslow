# 03: Retire the test-only local-JWT verifier

**What to build:** Local JWT verification exists in exactly one place, the authorizer. The organizer-level validation method that reads global configuration and is exercised only by tests is deleted, and its tests move to the authorizer with a settings snapshot and a controlled signing key. Token minting remains available for login flows.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] The organizer-level local-JWT validation method is deleted and no production or example code references it.
- [x] Tests formerly covering it assert the same outcomes through the authorizer: valid token, expired token, foreign signature, malformed token, and rejected signing method.
- [x] Local JWT minting used by login handlers is unchanged.
- [x] `go build ./...`, `go vet ./...`, `gofmt -l .`, and the full test suite pass.

## Comments

Resolved by merging `ticket/03-retire-local-jwt-verifier` (source commit `01ac0da`) into `identity-module-followups` as merge commit `0c512269c10f8fe81186a22c3165d72c055154e2`. No conflicts.

Verification run from the merge commit:

- `go build ./...` - pass (exit 0)
- `go vet ./...` - pass (exit 0)
- `gofmt -l .` - pass (no files listed)
- `go test ./...` - pass (`platform` ok, 1.930s; examples have no test files)
- `go test -race ./...` - pass (`platform` ok, 3.239s)

Additionally confirmed no remaining references to `ValidateLocalJwtToken` or `ErrLocalJwtInvalidToken`, and `NewLocalJwtToken` minting remains in use.
