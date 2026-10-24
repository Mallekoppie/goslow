# goslow platform

A runtime for hosting HTTP routes, Connect/gRPC surfaces, embedded web assets, and their authentication on one origin.

## Callers

**Identity**:
The caller a request has been proven to be, with the roles and claims attached to that proof. One authorizer resolves it per request, and handlers read it through the single `IdentityFromContext` accessor.
_Avoid_: user, principal, subject

**Credential**:
The proof a caller presents to establish an identity: a bearer token or a basic username and password.
_Avoid_: auth header, token

**Client token**:
The raw bearer token presented by a caller, retained only so it can be forwarded to an upstream service. It is not an identity. When forwarded, it is verified with the same key set and issuer as an ID token but without the audience check, because it was minted for a different client.
_Avoid_: access token, JWT

## Surfaces

**Route**:
An HTTP surface registered with a method and a path.
_Avoid_: endpoint, handler

**RPC procedure**:
A Connect, gRPC, or gRPC-Web surface identified by its fully qualified `/package.Service/Method` path.
_Avoid_: endpoint, RPC method

**Auth strategy**:
The named rule a protected surface declares for how callers may authenticate: `none`, `local-jwt`, `oauth`, `basic`, or `local-or-oidc`.
_Avoid_: auth mode, auth type, auth provider

**Required roles**:
The roles a protected surface demands; a caller holding any one of them passes the gate. A caller without one is forbidden, not unauthenticated: `403` on HTTP and `CodePermissionDenied` on RPC.
_Avoid_: allowed roles, permissions, scopes

**Unauthenticated exclusion list**:
The configured set of surfaces that are public without declaring `none`. A surface on this list that also declares a real strategy is a configuration error.
_Avoid_: whitelist, public paths, skip-auth list

**Access rule**:
The resolved policy for one surface: whether it is public and which roles it requires.
_Avoid_: ACL, permission, policy

## Verification

**ID token**:
An OIDC token minted by the identity provider and verified against that provider's published keys and the configured audience.
_Avoid_: OAuth token, access token

**Local JWT**:
A token issued and verified by this platform with a configured HMAC key, used as a bootstrap and development mechanism.
_Avoid_: platform token, internal token

**Roles claim**:
The token claim from which granted roles are read, given as a literal key or a dotted path.
_Avoid_: role field, groups claim

**Identity provider**:
The external OIDC issuer that mints ID tokens. Its unavailability is reported as unavailable, never as a rejected credential.
_Avoid_: auth server, auth service

## Auth outcomes

**Authentication required**:
No usable credential was presented, or the configured strategy definitely rejected it. Mapped to `401 Unauthorized` on HTTP and `CodeUnauthenticated` on RPC.
_Avoid_: unauthorized, unauthenticated error

**Forbidden**:
A verified caller that holds none of the required roles. Mapped to `403 Forbidden` on HTTP and `CodePermissionDenied` on RPC.
_Avoid_: unauthorized, not allowed

**Unavailable**:
The identity provider could not be reached and no credential was definitely rejected. Mapped to `503 Service Unavailable` with `Retry-After` on HTTP and `CodeUnavailable` on RPC.
_Avoid_: error, outage
