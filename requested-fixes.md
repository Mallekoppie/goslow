# goslow completion requirements — Connect transport

> **Historical record.** This document describes the work as it stood at the
> time it was written. Some symbols it names have since been removed or renamed;
> in particular, `Route.AuthRequired` and `routeAuthStrategies` were retired by
> [ADR-0001](docs/adr/0001-one-identity-module.md). Read the analysis below as
> history, not as current API.

## Status and context

This document is the remaining work for goslow after the Connect migration on
`fixes-for-wdid`. The migration to `connectrpc.com/connect` is complete and
intentional. `grpc-go` and the archived `improbable-eng/grpc-web` wrapper are no
longer part of the transport story and must not be reintroduced.

The platform must now be a complete, reusable runtime for:

- hosting Connect, native gRPC, gRPC-Web, and embedded web assets
- authentication and authorization for HTTP and RPC transports
- request logging, request IDs, recovery, and lifecycle management
- configuration loading, defaults, validation, and programmatic setup
- durable BoltDB access and snapshots

Read the repository first. The normal verification baseline is:

```bash
go vet ./...
go test ./...
go test -race ./...
gofmt -l .
go mod tidy -diff
```

Build and test every module under `example/` separately.

## Intentional decisions — do not undo

- **Connect is the transport.** `connectrpc.com/connect` serves Connect,
  native gRPC, and gRPC-Web from one handler.
- **Deprecated APIs stay removed.** Do not restore `GRPCService`,
  `StartGrpcServer`, `StartGrpcServerWithWeb`, the old
  `improbable-eng/grpc-web` dependency, or `github.com/boltdb/bolt`.
- **No compatibility shims.** Breaking changes are intentional. Update examples,
  tests, README, and consumers to the Connect API instead of adding deprecated
  wrappers.
- **No parallel REST/JSON transport** for capabilities that are exposed as RPCs.
- **No package `init()` side effects.** Configuration, logging, OAuth client
  renewal, and Vault initialization must be explicit or lazily idempotent.
- **No wildcard CORS in production.** Use exact origins for both HTTP and
  gRPC-Web.
- **Do not log secrets or full tokens.** This includes client tokens, JWTs,
  registration secrets, Vault tokens, and request bodies containing credentials.

## Baseline already implemented — do not redo

The following work is already in place on the branch. Treat it as the baseline,
not as requirements:

- Connect server with Connect/gRPC/gRPC-Web protocol tests.
- Streaming test coverage and unary/stream auth interceptors.
- gRPC-Web CORS preflight with an origin allow-list.
- h2c for plaintext native gRPC and TLS support.
- `bbolt` migration, `0600` file mode, open timeout, nil-safe close, and
  `View`/`Update`/`Snapshot`/`Integrity`/`Stats`/`Scan`/`ScanPrefix`/`Count`.
- HTTP graceful shutdown, server timeouts, request IDs, panic recovery, access
  logging, and bounded JSON request bodies.
- JWT token redaction, OAuth roles-claim extraction, and lazy OIDC discovery.
- Example modules building against the local platform.

## P0 — correctness and security

### P0.1 Make configuration initialization a single validated path

**Problem**

- `platform/vault.go` still dereferences a nil config after
  `GetPlatformConfiguration()` returns an error. A malformed `config.yml` panics
  during package initialization.
- `platform/config.go` `SetPlatformConfiguration` stores the supplied struct
  directly and never calls `checkPlatformConfiguration`. Programmatic
  configuration therefore has zero HTTP/gRPC timeouts, an empty log file path,
  and no OAuth-client or Vault initialization.
- `platform/oauth.go` `initializeOAuthTokenClients` reads the global
  `internalConfig` directly without synchronization and is not called when
  configuration is set programmatically.

**Required change**

- Make one validation path used by both file loading and programmatic setup.
  `SetPlatformConfiguration` must copy the caller's config, apply defaults,
  validate it, and store only valid configuration. Prefer returning an error
  (breaking changes are allowed) so callers cannot silently run with zero
  timeouts.
- Make Vault initialization tolerate `(nil, err)` from
  `GetPlatformConfiguration` and return without dereferencing nil. Do not mark
  Vault initialized when configuration loading failed.
- Initialize or reinitialize OAuth client renewal and Vault after a successful
  configuration change, or make both subsystems lazily resolve the current
  configuration. Do not depend on package `init()` ordering.
- Remove direct `internalConfig` reads from `initializeOAuthTokenClients`; obtain
  a locked snapshot through the public configuration accessor.
- If Vault secret retrieval fails, skip that OAuth client with a clear error
  instead of enabling it with empty credentials.
- Remove the remaining package `init()` side effects in `logging.go`,
  `oauth.go`, and `vault.go`. `marshalling.go`'s trivial init can also be
  removed.

**Acceptance criteria**

- A malformed config returns a descriptive error; the process does not panic.
- `SetPlatformConfiguration(Config{})` produces non-zero HTTP and gRPC
  timeouts, a usable log path when file logging is enabled, and valid defaults.
- Programmatic OAuth-client and Vault configuration is initialized without
  requiring a config file.
- Tests cover malformed config, programmatic defaults, Vault-disabled and
  Vault-enabled startup, and OAuth-client initialization.

### P0.2 Remove the logger data race and init-time side effect

**Problem**

`platform/logging.go` reassigns the global `Log` pointer while
`Info`/`Error`/`Debug`/`Warn` read it without synchronization. `loggerLock` only
guards writers. Concurrent reconfiguration and logging is a data race.
`InitializeLogger` is still called from `init()`.

**Required change**

- Keep the `Log` global pointer stable for the lifetime of the process and store
  the actual `*zap.Logger` in an `atomic.Pointer` (or equivalent atomic value)
  inside `Logger`.
- Make `ConfigureLoggerWithConfig` safe for concurrent calls.
- Validate the configured log level and surface `ErrInvalidLogLevel` instead of
  silently falling back to info.
- Remove `init()`-driven logger creation. Initialize the logger explicitly from
  the application/platform startup path, or lazily on first use with an
  idempotent, race-free path.
- Ensure every logging call remains safe when the logger has not yet been
  configured.

**Acceptance criteria**

- A race test that concurrently calls `Log.Info` and `ConfigureLogger` passes
  under `go test -race`.
- An invalid log level is reported to the caller.
- Importing the package does not read configuration or create a logger.

### P0.3 Make authentication fail closed

**Problem**

`platform/httpserver.go` allows `Route.AuthRequired: true` to become completely
public when no auth strategy is enabled. `routeAuthStrategies` returns an empty
strategy list and `newRouter` installs no middleware.

**Required change**

- Remove the legacy `AuthRequired` inference and its AND semantics. Require an
  explicit `Route.Auth` value for every route.
- `AuthNone` must remain the explicit way to declare a public route.
- If `Route.Auth` names a strategy that is not enabled or cannot be built,
  return a startup error; do not silently fall back to public.
- If both local JWT and OAuth are enabled, expose an explicit
  `local-or-oidc` strategy rather than relying on implicit AND/OR behavior.
- Update README and examples to show explicit auth on every route.

**Acceptance criteria**

- A route with no explicit auth does not silently become public.
- A route that requests a disabled strategy fails at startup with a clear error.
- Public routes require `Auth: AuthNone`.
- Tests cover `none`, `local-jwt`, `oauth`, `local-or-oidc`, and invalid
  combinations.

### P0.4 Give Connect the same request middleware as HTTP

**Problem**

The Connect handler installs only the auth interceptor. It has no panic
recovery, request ID propagation, access logging, or SLA timing. A panic becomes
an aborted connection rather than a proper `CodeInternal` response, and RPCs
produce no access logs or request correlation.

**Required change**

- Add default Connect interceptors for:
  - panic recovery, returning `connect.CodeInternal`
  - request ID generation/propagation (`X-Request-ID` response header and
    request context)
  - access logging with procedure, code, duration, and request ID
  - optional SLA logging consistent with `Route.SlaMs`
- Cover both unary and streaming handlers.
- Order interceptors so recovery and logging wrap authentication; auth failures
  must still be logged, and panics must still be recovered.
- Reuse the HTTP request-ID and recovery behavior where practical instead of
  duplicating logic.
- Never log tokens, authorization metadata, or request bodies.

**Acceptance criteria**

- A panicking RPC returns `CodeInternal` with a request ID.
- Unary and streaming RPCs produce access logs containing the request ID.
- Auth failures are logged once, without token values.
- A race test covers concurrent Connect requests and logging.

## P1 — platform completeness

### P1.1 Add health and reflection to Connect

**Required change**

- Register `grpc_health_v1` health checks for the server.
- Add server reflection for development and tests.
- Make reflection explicitly configurable and disabled by default in production.
- Prefer `connectrpc.com/grpchealth` and `connectrpc.com/grpcreflect`, keeping
  the Connect-only transport decision.

**Acceptance criteria**

- `grpcurl` can list services against a development server with reflection
  enabled.
- Health checks return serving/not-serving states correctly.
- Reflection is unavailable when disabled.

### P1.2 Add an HTTP CORS allow-list

**Problem**

gRPC-Web has `Grpc.Server.AllowedOrigins`, but HTTP routes still use
`AllowCorsForLocalDevelopment` with `Access-Control-Allow-Origin: *`,
`Access-Control-Allow-Methods: *`, and `Access-Control-Allow-Headers: *`.

**Required change**

- Add `HTTP.Server.AllowedOrigins` (or equivalent) and use an exact-origin CORS
  middleware for HTTP routes and preflight.
- Default `AllowCorsForLocalDevelopment` to false. Local development must opt in
  and specify the Flutter dev origin explicitly.
- Share CORS origin matching between HTTP and gRPC-Web; do not maintain two
  different allow-list implementations.
- Do not use wildcard origins with credentials.

**Acceptance criteria**

- Allowed origins receive the exact `Access-Control-Allow-Origin` value.
- Disallowed origins receive no CORS grant.
- Preflight works for the configured Flutter dev origin.
- Wildcard CORS is not enabled by default in production configuration.

### P1.3 Define Connect auth strategy and per-procedure authorization

**Problem**

Connect auth always ORs enabled local JWT and OAuth verifiers and has no
per-procedure roles. HTTP `AuthRequired` uses AND semantics, so the two
transports can enforce different policies. There is no equivalent of
`Route.RolesRequired` for RPCs.

**Required change**

- Add an explicit Connect auth strategy, using the same strategy vocabulary as
  HTTP (`none`, `local-jwt`, `oauth`, `local-or-oidc`, and optionally `basic` if
  it can be supported safely).
- Add per-procedure role requirements, for example a
  `map[string][]string` of procedure → roles.
- Enforce roles in the Connect interceptor and return `CodePermissionDenied` on
  failure.
- Keep `Grpc.Server.UnAuthenticatedPaths` as the exact public-method allow-list,
  but ensure it and the role policy are both applied to unary and streaming
  RPCs.
- Document the chosen OR/AND semantics explicitly.

**Acceptance criteria**

- A procedure can require no roles, one role, or multiple roles.
- A valid identity without the required role receives `CodePermissionDenied`.
- Local JWT and OAuth behavior is deterministic and documented.
- Tests cover unary and streaming paths, allowed roles, denied roles, and public
  methods.

### P1.4 Do not let unknown RPC paths fall through to the web handler

**Problem**

`withWebFallback` sends every path that does not match an RPC pattern to
`WebHandler`. With an SPA fallback, an unknown RPC path such as
`/wdid.v1.Missing/Method` returns `200` HTML instead of an RPC error.

**Required change**

- Detect RPC-shaped requests (gRPC/gRPC-Web/Connect content types,
  `Connect-Protocol-Version`, or reserved RPC path prefixes) and return `404` or
  a Connect error when no procedure matches.
- Send only genuinely non-RPC requests to `WebHandler`.
- If an SPA fallback is provided, ensure it applies only to non-RPC paths.

**Acceptance criteria**

- An unknown RPC path returns `404` or the appropriate Connect error, never
  `200` HTML.
- Static assets and SPA routes still work.
- Tests cover both an `http.FileServer` web handler and an SPA fallback.

### P1.5 Separate streaming timeouts from unary request timeouts

**Problem**

`newConnectHTTPServer` applies `Grpc.Server.WriteTimeoutSeconds` (default 60s) to
the whole HTTP response, including long-lived server streams. A valid stream can
be terminated by the HTTP write timeout.

**Required change**

- Add a dedicated stream/write timeout setting, or allow the Connect server write
  timeout to be zero for streaming deployments while keeping unary requests
  bounded.
- Document the behavior and the recommended production values.
- Ensure graceful shutdown still terminates streams within the shutdown timeout.

**Acceptance criteria**

- A server stream can remain open longer than the unary write timeout when
  configured to do so.
- Unary requests still have a bounded write timeout.
- Configuration tests cover the new setting.

### P1.6 Validate enabled features at startup

**Required change**

- Validate, with descriptive errors:
  - OAuth: issuer/well-known URL, client ID, and allowed algorithms when enabled
  - local JWT: signing key and signing method when enabled
  - TLS: certificate and key files when enabled
  - HTTP and gRPC listener addresses
  - log level
  - CORS origins
- Fail startup on invalid configuration instead of returning per-request `401`
  or `503` errors caused by missing configuration.
- Keep the intentional behavior where an unreachable IdP returns `503` at
  request time while other routes continue serving.

**Acceptance criteria**

- Missing OAuth fields, missing JWT keys, and missing TLS files fail startup
  with actionable errors.
- Provider outages still return `503` with `Retry-After` without panicking.
- Tests cover each invalid configuration case.

### P1.7 Harden OAuth client token retrieval

**Problem**

`internalGetOAuth2Token` ignores the `http.NewRequest` error and then calls
`request.Header.Add`, which panics when the token endpoint is empty or invalid.
OAuth client TLS verification is also disabled globally by default.

**Required change**

- Check and return the `http.NewRequest` error; validate the token endpoint
  before building the request.
- Use a request context and bounded timeout.
- Do not enable a client with empty Vault-derived values.
- Remove the global `InsecureSkipVerify: true` default. Make insecure TLS an
  explicit local-development option.
- Keep client secrets and tokens out of logs.

**Acceptance criteria**

- An empty or malformed token endpoint returns an error, not a panic.
- TLS verification is enabled by default.
- Insecure TLS is opt-in and documented.
- Tests cover missing endpoint, invalid URL, Vault errors, and successful token
  retrieval.

### P1.8 Provide one supported combined hosting entry point

**Problem**

`StartHttpServerWithWeb` serves HTTP routes plus static assets.
`StartConnectServerWithContext` serves Connect plus static assets. Consumers that
need legacy HTTP routes during migration, or a single application origin for
Connect and HTTP, have to assemble their own `http.Server` and duplicate
lifecycle code.

**Required change**

- Add a supported handler/server API that mounts, on one origin:
  - Connect, native gRPC, and gRPC-Web
  - optional `platform.Routes`
  - optional embedded `WebHandler`
  - request ID, recovery, and logging
- Provide graceful shutdown and configured timeouts.
- If a combined server is intentionally out of scope, document the supported
  composition pattern and provide a reusable helper instead of leaving consumers
  to copy `serveHTTPServer`.

**Acceptance criteria**

- One listener can serve an RPC, an HTTP route, and a static asset in tests.
- Graceful shutdown covers all protocols.
- No duplicate lifecycle implementation is required in examples.

### P1.9 Provide an SPA fallback helper

**Recommended change**

- Add an `SPAHandler` or `WithSPAFallback` helper that serves `index.html` for
  non-RPC, non-asset paths. Flutter path-based routing needs this.
- Keep RPC paths excluded per P1.4.

**Acceptance criteria**

- `/today` returns `index.html` when the file does not exist.
- `/wdid.v1.Service/Method` still returns an RPC error, not `index.html`.

### P1.10 Make the JSON body limit configurable

**Recommended change**

- Add `HTTP.Server.MaxRequestBodyBytes` and use it as the default for
  `ReadJsonRequest`.
- Keep `ReadJsonRequestLimit` for per-call overrides.
- Document `DisallowUnknownFields` as a deliberate behavior change.

**Acceptance criteria**

- Oversized bodies return the documented error.
- The configured limit is applied.
- The default remains safe (1 MiB unless configured).

## P2 — cleanup and quality

- Remove `staticcheck` findings and add `staticcheck ./...` to CI:
  - capitalized error strings in `config.go`, `database.go`, `vault.go`
  - string context keys in `middleware.go` (use typed keys)
  - `io/ioutil` in `oauth.go` and `vault.go`
  - unused `oAuthClientTokenEnabled`
  - unused `echoService.authPrefix`
  - `for true` in the forward-client-token example
- Make `Makefile`'s `vet` target fail when `gofmt -l .` is non-empty.
- Fix `StartHttpServerWithWeb` to return the `fs.Sub` error instead of passing a
  potentially nil filesystem to `http.FS`.
- Remove full JWT logging from `example/grpc-auth/client/main.go`; omit or
  redact the token.
- Fix `example/grpc-auth/hello/hello.proto`: `option go_package = "/gen";` is
  not a valid Go import path.
- Remove trailing whitespace from `readme.md` so `git diff --check` passes.
- Redact or summarize objects in `SaveObject` debug logs; journal content is
  sensitive.
- Rotate/remove the pre-existing Vault token and client secret in
  `platform/config.yml`; use placeholders.
- Update the README for:
  - Connect logging, request IDs, recovery, health, and reflection
  - explicit auth on every route
  - HTTP and gRPC CORS configuration
  - programmatic configuration defaults and validation
  - streaming timeout behavior
  - the removal of the old gRPC API and the Connect migration
- Replace deprecated `github.com/dgrijalva/jwt-go` with
  `github.com/golang-jwt/jwt/v5` and remove the old dependency.
- Consider upgrading `connectrpc.com/connect` to the latest patch release,
  `google.golang.org/protobuf` to the latest patch release, and `bbolt` to the
  current maintained line after compatibility tests. Consider `coreos/go-oidc/v3`
  for the OIDC dependency.

## Verification and definition of done

Run at minimum:

```bash
go vet ./...
go test ./...
go test -race ./...
gofmt -l .          # must be empty
go mod tidy -diff   # must be empty
staticcheck ./...
git diff --check
```

Then build and test each module under `example/` separately.

The work is done when:

- All P0 and P1 items are implemented with tests.
- P2 items are either completed or explicitly deferred with a reason.
- No deprecated API or parallel transport was reintroduced.
- HTTP, Connect, native gRPC, and gRPC-Web all share consistent auth, logging,
  recovery, request-ID, and CORS behavior.
- Configuration failures are detected at startup, not per request.
- The platform can host the Flutter web app and the RPC surface on one origin
  without consumer-side lifecycle duplication.
- `README.md`, `platform/config.yml`, examples, and the devcontainer remain in
  sync with the implementation.

## Suggested implementation order

1. Configuration lifecycle and logger concurrency (P0.1, P0.2).
2. Fail-closed auth and explicit route policies (P0.3).
3. Connect request middleware: recovery, request ID, logging (P0.4).
4. Health, reflection, and unknown-RPC routing (P1.1, P1.4).
5. HTTP CORS, auth policy, validation, and streaming timeouts (P1.2, P1.3,
   P1.5, P1.6).
6. OAuth client hardening and combined hosting (P1.7, P1.8, P1.9, P1.10).
7. Cleanup, docs, and CI gates (P2).
