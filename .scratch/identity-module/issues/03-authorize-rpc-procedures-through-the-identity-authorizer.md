# 03: Authorize RPC procedures through the identity authorizer

**What to build:** Connect, gRPC, and gRPC-Web procedures authorized through the same identity authorizer as HTTP, for unary and streaming handlers. No fabricated HTTP request is involved, required roles follow the configured roles claim, basic auth is rejected at construction, and the combined application shares one authorizer and provider cache across both adapters.

**Blocked by:** 02: Authorize HTTP routes through the identity authorizer

**Status:** ready-for-agent

- [ ] Connect, gRPC, and gRPC-Web unary procedures authorize through the authorizer; streaming handler procedures do too.
- [ ] No fabricated HTTP request is used for authentication.
- [ ] Procedure roles come from the configured roles claim with any-of matching; a caller lacking one receives `CodePermissionDenied`.
- [ ] A missing or invalid credential returns `CodeUnauthenticated`; an identity-provider outage with no definite rejection returns `CodeUnavailable`.
- [ ] A basic strategy for RPC fails at handler construction; a procedure on the unauthenticated exclusion list is public.
- [ ] The combined application builds one authorizer shared by the HTTP and RPC adapters, with one provider cache.
- [ ] Auth failures still carry the request ID and are logged once without token values.
- [ ] Protocol tests for Connect, gRPC, gRPC-Web, and streaming pass.
