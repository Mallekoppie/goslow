# 02: Authorize HTTP routes through the identity authorizer

**What to build:** A running HTTP server where every route is authorized through the identity authorizer. Rules are resolved once when the router is built, the unauthenticated exclusion list is applied there, a verified caller lacking a required role receives 403, and handlers read the caller through the single identity accessor.

**Blocked by:** 01: Add the identity authorizer

**Status:** resolved

- [x] Every HTTP route resolves an access rule once at router construction; the unauthenticated exclusion list is applied there.
- [x] A route outside the exclusion list with no auth strategy fails router construction; an excluded route declaring a real strategy also fails.
- [x] A route requesting a disabled strategy fails router construction.
- [x] Public routes perform no credential work.
- [x] A valid credential lacking a required role returns 403; a missing or invalid credential returns 401; an identity-provider outage with no definite rejection returns 503 with `Retry-After`.
- [x] Required roles are enforced for `local-jwt`, `oauth`, and `local-or-oidc` routes.
- [x] Handlers read the caller through the single identity accessor; the HTTP examples are migrated.
- [x] Access logging and metrics still record the decision, without the old auth-outcome context key.
- [x] HTTP routes served by the combined application use the authorizer.
- [x] Existing HTTP protocol and policy tests pass; RPC behaviour is unchanged.

## Comments

Completed in `45b7ec6`. Every HTTP route resolves its access rule at router
construction against a settings snapshot and authorizes through the identity
authorizer in `platform/httpauth.go`; handlers read `IdentityFromContext`, and
the HTTP examples are migrated. Verified with `go test ./...`,
`go test -race ./platform`, `go vet ./...`, `gofmt -l .`, and
`make build-examples`.
