# 07: Add run readmes for the HTTP examples

**What to build:** The web-server, local-JWT, and HTTP API examples each have a readme that takes a client from a clean checkout to a working request. Each covers prerequisites, the configuration it reads, the run command, its routes, and the expected authentication responses, including any placeholder identity-provider setting and the role required by the protected route.

**Blocked by:** 04: Make the HTTP examples honour the identity contract; 05: Reject discovery-document URLs in the OAuth issuer setting

**Status:** resolved

- [x] Each of the three examples has a readme with prerequisites, configuration location, run command, route list, and expected auth outcomes.
- [x] The web-server readme labels the placeholder issuer base and explains what to replace it with; following it against a real provider yields a working OAuth route.
- [x] The local-JWT readme demonstrates login, then an authenticated request with the returned token.
- [x] The HTTP API readme states that it has no protected routes.

## Comments

Merged from `ticket/07-add-http-example-readmes` at source commit `05d2ef51135f01e90c64afc0255e8ce9e49811ac` in merge commit `655ea880a8ba2ee7e68aa77bb5881891936ccc4a` (no conflicts).

Verification performed from the integration branch: `go build ./...`, `go vet ./...`, `gofmt -l .` (no output), `go test -count=1 ./...`, `go test -count=1 -race ./...`, `make build-examples`, plus the `example/http-web-server` and `example/localJwt` module test suites. All passed.
