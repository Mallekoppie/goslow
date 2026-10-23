# goslow platform

A runtime for hosting HTTP routes, Connect/gRPC surfaces, embedded web assets, and their authentication on one origin.

## Callers

**Identity**:
The caller a request has been proven to be, with the roles and claims attached to that proof.
_Avoid_: user, principal, subject

**Credential**:
The proof a caller presents to establish an identity: a bearer token or a basic username and password.
_Avoid_: auth header, token

**Client token**:
The raw bearer token presented by a caller, retained only so it can be forwarded to an upstream service. It is not an identity.
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
The roles a protected surface demands; a caller holding any one of them passes the gate. A caller without one is forbidden, not unauthenticated.
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
