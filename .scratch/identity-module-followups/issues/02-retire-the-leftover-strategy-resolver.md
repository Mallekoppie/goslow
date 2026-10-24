# 02: Retire the leftover strategy resolver and test-only interceptor

**What to build:** The one-module auth decision has no leftover parallel rule resolution. The production-dead strategy resolver and the example that printed its retired any-of `local-or-oidc` expansion are gone, and tests that need a Connect interceptor use the same authorizer-backed construction as production.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] No production-dead strategy resolver remains; rule resolution exists only in the authorizer.
- [ ] The example output that still expands `local-or-oidc` into two strategies is removed.
- [ ] No test-only Connect interceptor constructor remains, or any remaining one delegates to the authorizer-backed constructor.
- [ ] Protocol coverage for Connect, gRPC, gRPC-Web, unary, and streaming still passes.
- [ ] `go build ./...`, `go vet ./...`, `gofmt -l .`, and the full test suite pass.
