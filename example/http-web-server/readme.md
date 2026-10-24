# HTTP Web Server Example

Serves platform HTTP routes and an embedded static site from one listener. Most
routes are public; `/oauth2` is protected by OAuth and requires a verified OIDC
ID token whose roles include `read`.

## Prerequisites

- Go 1.27.2. The module resolves the platform through a `replace` directive, so
  no separate install is needed.
- For `/oauth2`: an OIDC identity provider that mints ID tokens for this
  example.
- For `/secrets`: a reachable Vault server holding the configured secret path.
  The other routes run without Vault.

## Configuration

`example/http-web-server/config.yml` is read from the current working
directory, so run the example from its own directory. The settings that matter
for a first run are:

| Setting | Checked-in value | Meaning |
| --- | --- | --- |
| `platform.http.server.listeningaddress` | `0.0.0.0:8080` | Listener address |
| `platform.auth.server.oauth.enabled` | `true` | Enables the OAuth strategy |
| `platform.auth.server.oauth.idpwellknownurl` | `https://idp.example` | Placeholder issuer base (see below) |
| `platform.auth.server.oauth.clientid` | `example-client` | Expected token audience |
| `platform.auth.server.oauth.rolesclaim` | `roles` (default) | Claim read for roles |
| `platform.database.boltdb.filename` | `./database.db` | Bolt database file |
| `platform.vault.addresslist` | `https://localhost:8200` | Vault used by `/secrets` |

### Replace the identity-provider placeholder

`platform.auth.server.oauth.idpwellknownurl: "https://idp.example"` is a
placeholder **issuer base URL**. Replace it with the issuer base of your
identity provider, for example `https://login.example.com` or
`https://<instance>.zitadel.cloud`.

Do not include `/.well-known/openid-configuration`; the platform appends the
discovery-document path itself, and startup validation rejects a value that
already contains it. Also set `clientid` to the audience the provider mints ID
tokens for, and `allowedalgorithms` to the provider's signing algorithms.

With the placeholder in place, discovery cannot resolve, so `/oauth2` reports
the identity provider as unavailable for any token that is presented.

## Run

```bash
cd example/http-web-server
go run .
```

The server logs `Starting new HTTP server` with `ListeningAddress
0.0.0.0:8080`. Stop it with Ctrl+C.

## Routes

| Method | Path | Auth | Behaviour |
| --- | --- | --- | --- |
| `GET` | `/` | none | Serves the embedded `dist/index.html` |
| `POST` | `/db/write` | none | Stores `{"id":"...","name":"...","surname":"..."}`; `200` on success, `400` on malformed JSON |
| `POST` | `/db/read` | none | Reads `{"id":"..."}`; `200` with the object, `204` when absent, `400` on malformed JSON |
| `GET` | `/config` | none | Returns the `componentconfigexample` section as JSON |
| `GET` | `/all` | none | Returns every stored object |
| `GET` | `/secrets` | none | Reads `kv-v2/data/dev/test/creds` from Vault |
| `GET` | `/oauth2` | `oauth`, role `read` | Returns the verified caller's username, issuer, and roles |

Public routes declare `Auth: platform.AuthNone` explicitly. The static handler
is mounted as the web fallback by `StartHttpServerWithWeb`.

## Example requests

```bash
curl -i http://127.0.0.1:8080/
curl -i http://127.0.0.1:8080/config

curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"id":"1","name":"Ada","surname":"Lovelace"}' \
  http://127.0.0.1:8080/db/write

curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"id":"1"}' \
  http://127.0.0.1:8080/db/read
```

These return `200` (`/db/write` has an empty body). Asking for one object
returns the stored JSON; `/config` returns `{"Name":"my custom config","Port":"9001"}`.

`/secrets` returns `500` unless the configured Vault server is reachable.

## Authentication outcomes for `/oauth2`

The route requires the `read` role (declared once as `service.OAuthRequiredRole`
and used by both the route and the handler).

```bash
# No credential
curl -i http://127.0.0.1:8080/oauth2
# HTTP/1.1 401 Unauthorized

# A token presented while the placeholder issuer cannot be resolved
curl -i -H 'Authorization: Bearer not-a-token' http://127.0.0.1:8080/oauth2
# HTTP/1.1 503 Service Unavailable
# Retry-After: 10
```

Once a real issuer is configured:

| Caller | HTTP result | Body |
| --- | --- | --- |
| No credential | `401 Unauthorized` | Rejected by the authorizer |
| Invalid or expired ID token | `401 Unauthorized` | Rejected by the authorizer |
| Verified token lacking `read` | `403 Forbidden` | `Forbidden` |
| Verified token holding `read` | `200 OK` | `{"username":"...","issuer":"...","roles":[...]}` |
| Identity provider unreachable | `503 Service Unavailable` + `Retry-After` | Provider failure is not reported as a rejected credential |

The `403` and success paths are covered directly by
`service/service_test.go`; the `401` and `503` paths were observed against the
checked-in placeholder configuration.
