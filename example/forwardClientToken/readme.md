# Overview
This example shows how a client token can be forwarded to a remote API.

# Setup
Set up IDP clients for the client and server services, then update each
`config.yml` with the realm URL and client IDs:
- `client/config.yml` authorizes callers issued to `federate-token-client` and
  requests its own token as `federate-token-server`.
- `server/config.yml` authorizes callers issued to `federate-token-server`.

# Process
* Dialer gets token for client service
* Dialer calls client service
* Client service validates token
* Client service gets own token
* Client service calls server service and attaches the client token
* Server service validates token and then also validates the original client token
* Server service can now do authorization based on original client token

# Audience policy
The server's own route token must be issued to its configured client ID
(`federate-token-server`). The forwarded client token was issued to a different
client (`federate-token-client`), so `ValidateClientToken` verifies its
signature, expiry, and issuer but deliberately skips the audience check.
Requiring the server's client ID there would reject every forwarded token.

Route authorization does enforce the audience check: the token's `aud` claim
must contain the client ID configured for the service that receives it.

## Keycloak audience mapper
Keycloak does not put the requesting client's ID in `aud` by default. Stock
Keycloak issues tokens whose audience is `account` (plus other built-in
audiences), so a valid token with the `user` role still fails the route's
audience check and returns `401 Unauthorized`.

Add an **Audience** protocol mapper to each client (or a realm-level mapper that
applies to both) with `Included Custom Audience` set to the service's client ID:

- `federate-token-client` must be included in the `aud` of tokens the dialer
  presents to the client service.
- `federate-token-server` must be included in the `aud` of the client service's
  own route token, which the server service verifies.

For example, a token accepted by the server service contains:

```json
"aud": ["account", "federate-token-server"]
```

The forwarded client token still carries `federate-token-client` in `aud`; that
is expected, and `ValidateClientToken` deliberately ignores it.

# Required roles and Keycloak realm roles
Both routes require the `user` role (`RolesRequired: []string{"user"}`), so a
token that verifies without that role returns `403 Forbidden`.

Keycloak puts realm roles under `realm_access.roles` by default:

```json
"realm_access": {
  "roles": ["offline_access", "uma_authorization", "user"]
}
```

Both `client/config.yml` and `server/config.yml` therefore set:

```yaml
platform:
  auth:
    server:
      oauth:
        rolesclaim: realm_access.roles
```

For a valid token to pass the role gate:

- Assign the `user` realm role to the user (directly or through a group) in the
  realm that issues the token.
- Keep `rolesclaim: realm_access.roles` for Keycloak realm roles. For client
  roles use `resource_access.<client-id>.roles`, or add a Keycloak protocol
  mapper that emits a top-level `roles` claim and set `rolesclaim: roles`.
- `rolesclaim` is matched as a literal claim key first, then as a dotted path,
  and accepts either a list of strings or an object whose keys are the roles.

Without a matching claim the verified caller holds no roles, which is a
`403 Forbidden` rather than an authentication failure.
