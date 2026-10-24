# 02: Retire the leftover strategy resolver and test-only interceptor

**What to build:** The one-module auth decision has no leftover parallel rule resolution. The production-dead strategy resolver and the example that printed its retired any-of `local-or-oidc` expansion are gone, and tests that need a Connect interceptor use the same authorizer-backed construction as production.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] No production-dead strategy resolver remains; rule resolution exists only in the authorizer.
- [x] The example output that still expands `local-or-oidc` into two strategies is removed.
- [x] No test-only Connect interceptor constructor remains, or any remaining one delegates to the authorizer-backed constructor.
- [x] Protocol coverage for Connect, gRPC, gRPC-Web, unary, and streaming still passes.
- [x] `go build ./...`, `go vet ./...`, `gofmt -l .`, and the full test suite pass.

## Comments

Merged `ticket/02-retire-strategy-resolver` (source commit `203f589`) into `identity-module-followups` with a clean `--no-ff` merge (merge commit `18c6050`, no conflicts).

Verification on the merge result: `go build ./...` passed, `go vet ./...` passed, `gofmt -l .` reported no files, `go test ./...` passed (`platform` 2.017s), and `go test -race ./...` passed (`platform` 3.442s).
