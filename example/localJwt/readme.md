# Local JWT Example

Shows the platform's local JWT flow: a public login route mints a token, and two
protected routes accept that token. Local JWTs are issued and verified by the
platform with the configured HMAC key; they are a bootstrap and development
mechanism, not a replacement for an identity provider.

## Prerequisites

- Go 1.27.2. The module resolves the platform through a `replace` directive, so
  no separate install is needed.
- `curl` for the walkthrough below.

## Configuration

`example/localJwt/config.yml` is read from the current working directory, so run
the example from its own directory.

| Setting | Checked-in value | Meaning |
| --- | --- | --- |
| `platform.http.server.listeningaddress` | `0.0.0.0:8080` | Listener address |
| `platform.auth.server.localjwt.enabled` | `true` | Enables local JWT |
| `platform.auth.server.localjwt.jwtsigningkey` | `my super secret key` | HMAC signing key |
| `platform.auth.server.localjwt.jwtsigningmethod` | `HS256` | Signing algorithm |
| `platform.auth.server.localjwt.jwtexpiration` | `1` | Token lifetime in minutes |
| `platform.auth.server.localjwt.rolesclaim` | `roles` (default) | Claim read for roles |

Because the lifetime is one minute, an expired token is easy to produce: log in,
wait, then retry the protected request.

## Run

```bash
cd example/localJwt
go run .
```

The server logs `Starting new HTTP server` with `ListeningAddress
0.0.0.0:8080`. Stop it with Ctrl+C.

## Routes

| Method | Path | Auth | Behaviour |
| --- | --- | --- | --- |
| `POST` | `/login` | none | Accepts `{"username":"test","password":"test"}` and returns a JWT as plain text |
| `GET` | `/query` | `local-jwt` | Describes the authenticated caller |
| `GET` | `/renew` | `local-jwt` | Issues a new token for the authenticated caller |

`/login` is deliberately public (`Auth: platform.AuthNone`); the two protected
routes declare `Auth: platform.AuthLocalJWT`.

## Walkthrough: log in, then call a protected route

Start the server, then in another terminal:

```bash
TOKEN=$(curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"username":"test","password":"test"}' \
  http://127.0.0.1:8080/login)

echo "$TOKEN"
# eyJhbGciOiJIUzI1NiIs... (a signed local JWT)
```

Send the token as a bearer credential:

```bash
curl -i -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/query
# HTTP/1.1 200 OK
# Query successful for user: test
```

The token carries `username: test` and `roles: ["user"]`. `/renew` accepts the
same header and returns another token:

```bash
curl -i -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/renew
# HTTP/1.1 200 OK, body: a new JWT
```

If the new token is minted within the same second as the old one, its claims and
therefore its encoded value can be identical.

## Authentication outcomes

| Call | Result |
| --- | --- |
| `POST /login` with `test`/`test` | `200 OK`, body is the JWT |
| `POST /login` with any other credentials | `401 Unauthorized`, `Invalid login credentials` |
| `GET /query` or `/renew` without `Authorization` | `401 Unauthorized`, `Unauthorized` |
| `GET /query` or `/renew` with an invalid or expired token | `401 Unauthorized`, `Unauthorized` |
| `GET /query` with a valid token | `200 OK`, `Query successful for user: test` |

A protected handler that runs without an identity fails closed with
`401 Unauthorized`; it never reports success. There is no refresh token, so an
expired caller logs in again.
