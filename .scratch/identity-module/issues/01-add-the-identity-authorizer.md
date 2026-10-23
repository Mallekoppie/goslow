# 01: Add the identity authorizer

**What to build:** A constructable authorizer that resolves an access rule and authorizes a credential end-to-end, returning one Identity. It covers public surfaces, local JWTs, ID tokens, and basic credentials, takes the first fully authorized credential for `local-or-oidc`, applies required roles, and reports the agreed failure categories. It works through its own interface against a fake identity provider and is not wired into any server yet.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] A public access rule authorizes without reading a credential, storing an identity, or contacting the identity provider.
- [x] A local JWT is verified against the settings snapshot's key and method; its roles come from the configured local-JWT roles claim, falling back to `role` only when the configured claim is absent.
- [x] An ID token is verified through the injected verifier factory; its roles come from the configured OAuth roles claim.
- [x] A basic credential is verified against the snapshot's bcrypt users.
- [x] Required roles are any-of and exact; a verified caller without one is forbidden, not unauthenticated.
- [x] A `local-or-oidc` rule takes the first fully authorized credential and never merges identities.
- [x] When nothing succeeds, a definite rejection yields authentication-required, an outage with no definite rejection yields unavailable, and a role denial yields forbidden.
- [x] A verifier can report "not applicable", so a credential meant for another strategy is not counted as a definite rejection.
- [x] An access rule for a disabled or unknown strategy, `none` with required roles, or `basic` with required roles fails at rule construction.
- [x] The authorizer stores exactly one identity per request; handlers read it through one accessor.
- [x] Auth settings are copied at construction; the request path reads no global configuration.
- [x] The module imports neither `net/http` nor the Connect package.
- [x] Tests construct the authorizer directly, inject a fake verifier, and pass without a router, server, config file, or real identity provider.
- [x] The existing HTTP and RPC behaviour still passes unchanged.

## Comments

Completed in `f94ca24`. Added the transport-neutral authorizer and its direct
tests in `platform/identity.go` and `platform/identity_test.go`; it is not wired
into a server. Verified with `go test ./...`, `go test -race ./platform`,
`go vet ./...`, and `gofmt -l .`.
