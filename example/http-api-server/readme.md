# HTTP API Server Example

A REST-style HTTP server backed by BoltDB. It has **no protected routes**: every
route declares `Auth: platform.AuthNone`, so no credentials are required and no
OAuth or local-JWT settings are needed.

## Prerequisites

- Go 1.27.2. This directory has no `go.mod` of its own; it builds as part of the
  root module and uses the platform source in this repository, so no separate
  install is needed. `go run .` still works from this directory.
- `curl` for the example calls.
- For `/secrets`: a reachable Vault server holding the configured secret path.
  The other routes run without Vault.

## Configuration

`example/http-api-server/config.yml` is read from the current working directory,
so run the example from its own directory.

| Setting | Checked-in value | Meaning |
| --- | --- | --- |
| `platform.http.server.listeningaddress` | `0.0.0.0:9111` | Listener address |
| `platform.auth.server.oauth.enabled` | `false` | OAuth is disabled and unused |
| `platform.database.boltdb.enabled` | `true` | Enables BoltDB |
| `platform.database.boltdb.filename` | `./database.db` | Bolt database file |
| `platform.vault.addresslist` | `https://localhost:8200` | Vault used by `/secrets` |

`main.go` explicitly opens the Bolt database before starting the server and
closes it on shutdown. Delete `database.db` to start from empty storage.

If port `9111` is already in use, change
`platform.http.server.listeningaddress` and use the new address below.

## Run

```bash
cd example/http-api-server
go run .
```

The server logs `Starting new HTTP server` with `ListeningAddress
0.0.0.0:9111`. Stop it with Ctrl+C.

## Routes

| Method | Path | Auth | Behaviour |
| --- | --- | --- | --- |
| `GET` | `/` | none | Returns `Hello World` |
| `POST` | `/db/write` | none | Stores `{"id":"...","name":"...","surname":"..."}`; `200` on success, `400` on malformed JSON |
| `POST` | `/db/read` | none | Reads `{"id":"..."}`; `200` with the object, `204` when absent, `400` on malformed JSON |
| `GET` | `/config` | none | Returns the `componentconfigexample` section as JSON |
| `GET` | `/all` | none | Returns every stored object |
| `GET` | `/secrets` | none | Reads `kv-v2/data/dev/test/creds` from Vault |

None of these routes declares an auth strategy other than `AuthNone`, and the
configuration enables no auth strategy. There is nothing to log in to and no
role to hold.

## Example requests

```bash
curl -i http://127.0.0.1:9111/
# HTTP/1.1 200 OK
# Hello World

curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"id":"1","name":"Ada","surname":"Lovelace"}' \
  http://127.0.0.1:9111/db/write
# HTTP/1.1 200 OK (empty body)

curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"id":"1"}' \
  http://127.0.0.1:9111/db/read
# HTTP/1.1 200 OK
# {"id":"1","name":"Ada","surname":"Lovelace"}

curl -i http://127.0.0.1:9111/config
# HTTP/1.1 200 OK
# {"Name":"my custom config","Port":"9001"}

curl -i http://127.0.0.1:9111/all
# HTTP/1.1 200 OK
# {"id":"1","name":"Ada","surname":"Lovelace"}
```

## Authentication outcomes

Because there are no protected routes, every route returns its normal result
whether or not an `Authorization` header is present. An unrecognised token is
ignored rather than rejected.

The one route with an external dependency is `/secrets`: it returns `500` unless
the configured Vault server is reachable at `https://localhost:8200` and the
`kv-v2/data/dev/test/creds` path exists.
