# 03: Authorize RPC procedures through the identity authorizer

**What to build:** Connect, gRPC, and gRPC-Web procedures authorized through the same identity authorizer as HTTP, for unary and streaming handlers. No fabricated HTTP request is involved, required roles follow the configured roles claim, basic auth is rejected at construction, and the combined application shares one authorizer and provider cache across both adapters.

**Blocked by:** 02: Authorize HTTP routes through the identity authorizer

**Status:** resolved

- [x] Connect, gRPC, and gRPC-Web unary procedures authorize through the authorizer; streaming handler procedures do too.
- [x] No fabricated HTTP request is used for authentication.
- [x] Procedure roles come from the configured roles claim with any-of matching; a caller lacking one receives `CodePermissionDenied`.
- [x] A missing or invalid credential returns `CodeUnauthenticated`; an identity-provider outage with no definite rejection returns `CodeUnavailable`.
- [x] A basic strategy for RPC fails at handler construction; a procedure on the unauthenticated exclusion list is public.
- [x] The combined application builds one authorizer shared by the HTTP and RPC adapters, with one provider cache.
- [x] Auth failures still carry the request ID and are logged once without token values.
- [x] Protocol tests for Connect, gRPC, gRPC-Web, and streaming pass.

## Comments

Completed in `cd7b403`. The RPC interceptor in `platform/connectauth.go` now
resolves per-procedure rules against one authorizer and extracts credentials
directly from headers; `NewApplicationHandler` builds that authorizer once and
passes it to both adapters. Covered by `platform/connectauth_identity_test.go`
for all three protocols, streaming, role/claim matching, error precedence,
construction-time failures, logging, and the combined application. Verified
with `go test ./...`, `go test -race ./platform`, `go vet ./...`, `gofmt -l .`,
and `make build-examples`.
