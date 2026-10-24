# One identity module owns request authentication and authorization

HTTP routes and Connect/gRPC procedures authenticated through two parallel paths: an HTTP-request-shaped verifier chain that Connect reused by fabricating a request, plus a separate role checker. They diverged — `RolesRequired` was ignored for local JWTs, Connect hard-coded the roles claim, and a role failure returned 401 on HTTP but `CodePermissionDenied` on RPC. We consolidated both into one transport-neutral authorizer: adapters extract a `Credential`, resolve an `AccessRule` at construction, call `Authorize`, and read one `Identity` from context. The module imports neither `net/http` nor `connect`; adapters own headers, status codes, and RPC codes.

## Considered options

- **Keep two adapters over shared helpers.** Rejected: rule resolution, failure precedence, and role extraction would still be written twice — the drift this decision exists to remove.
- **A flexible verifier registry** with custom strategies, custom credential schemes, a replaceable role mapper, and per-rule check functions. Rejected: the only seam with more than one adapter today is OIDC verification (production provider, test fake, client-token path). The extension points would be unexercised surface and permanent compatibility commitments.
- **Read configuration per request** inside the module. Rejected: construction-time settings make behaviour deterministic, remove hidden global reads from the request path, and let tests build one value instead of resetting package state.

## Consequences

- One auth implementation, the identity module, owns authentication and authorization on both transports. The former verifier chain, any/all middleware, hard-coded claim extraction, test-only middleware wrappers, five identity accessors, and their context keys are deleted.
- Failure categories map the same way everywhere: a missing or invalid credential is `401` / `CodeUnauthenticated`; a verified caller lacking a required role is `403` / `CodePermissionDenied`; an identity-provider outage with no definite rejection is `503` with `Retry-After` / `CodeUnavailable`.
- `local-or-oidc` takes the first fully authorized credential; identities from several verifiers are no longer merged.
- `RolesRequired` applies to local JWTs, and declaring roles on a `none` or `basic` rule fails at construction.
- Auth settings are frozen when a router or handler is built; changing them requires a new authorizer.
- The five individual identity context accessors are replaced by `IdentityFromContext`; `RequestIDFromContext` stays and works on public and authenticated requests.
- `OAuth.ValidateClientToken` shares the same verifier implementation and gains an explicit audience policy.
