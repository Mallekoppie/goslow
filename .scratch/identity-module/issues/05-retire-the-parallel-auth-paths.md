# 05: Retire the parallel auth paths

**What to build:** One auth implementation remains. The old verifier chain, merging middleware, hardcoded claim extraction, second keyset stack, test-only wrappers, old context keys, and five identity accessors are deleted. Documentation describes the one-module behaviour, and the full verification baseline passes.

**Blocked by:** 03: Authorize RPC procedures through the identity authorizer; 04: Verify client tokens through the shared ID-token verifier

**Status:** ready-for-agent

- [ ] The old verifier chain, any/all middleware, hardcoded claim extractors, second keyset stack, test-only middleware wrappers, old context keys, and five identity accessors are deleted.
- [ ] No production or example code reads roles, username, issuer, claims, or client token through anything but the single identity accessor.
- [ ] `RequestIDFromContext` remains and works on public and authenticated requests.
- [ ] README, glossary, and the ADR describe the one-module behaviour and the 401/403/503 mapping.
- [ ] `go vet ./...`, `go test ./...`, `go test -race ./...`, `gofmt -l .`, and every example build pass.
- [ ] A search for the removed helpers and context keys returns nothing.
