## Plan: goslow remediation (P0–P3)

Turn `requested-fixes.md` into ten sequential, independently testable phases. Phase 0 establishes a hermetic test baseline, Phase 1 migrates to bbolt first (so the new `View`/`Update` API doesn't expose a soon-to-be-replaced tx type), then P0 → P1 → P2 → P3. Breaking changes are allowed, but `SetupBoltDB`, `ReadJsonRequest`, `ReadAllObjects` and `AuthRequired` are retained where the cost is trivial.

**Steps**

### Phase 0 — green baseline (*blocks everything*)
1. Add `platform/testhelpers_test.go`: `resetPlatformState(t)` (restores `internalConfig`, `dbBolt`, vault/OIDC caches, `viper.Reset()`), `seedConfig(t)`, `withTestDB(t)` using `t.TempDir()`. No `t.Parallel` in global-touching tests.
2. Gate live-service tests behind `//go:build integration`: `TestGetOAuth2Token` (`platform/oauth_test.go`), `TestLoginWithToken` (`platform/vault_test.go`).
3. Rewrite `platform/boltdb_test.go` — it currently never opens a DB because `database.go`'s `init()` shadows the global.
4. Fix `example/localJwt/main.go`: it references `platform.Logger`, which does not exist.

### Phase 1 — bbolt migration (P2.4)
5. Replace `github.com/boltdb/bolt` with `go.etcd.io/bbolt` (v1.3.x, aliased as `bolt`) in `platform/boltdb.go`; `go mod tidy` the root and all 7 example modules with `replace` directives.
6. Prove on-disk compatibility by running the existing DB tests unchanged.

### Phase 2 — P0 BoltDB
7. `platform/boltdb.go`: `boltFileMode = 0o600`, `defaultOpenTimeout = 5s`, new `OpenBoltDB(path, timeout)` with `MkdirAll(filepath.Dir(path), 0o750)`, `bolt.Options{Timeout: …}`; `SetupBoltDB` delegates via `GetPlatformConfiguration()`.
8. Delete `databaseHackToRestartServiceIKnowThisIsBad` and its call — no chmod, no `.lock` handling.
9. `describeBoltOpenError` in `boltdb_unix.go` / `boltdb_windows.go`: path, `mode=%04o`, uid/gid, wraps the original error; special-case `bbolt.ErrTimeout` as "locked by another process".
10. Nil-safe, restartable `Close()`; guard `dbBolt` with a mutex.
11. Config field `Database.BoltDB.OpenTimeoutSeconds`; acceptance tests for permissions, reopen, lock timeout, missing config, and error context.

### Phase 3 — quick correctness (P1.1, P1.2)
12. `ReadObject`: nil-handle guard and `b == nil` → `ErrNoEntryFoundInDB`.
13. `localJwtAuthMiddleware`: strict `"Bearer "` prefix check + trimmed token; shared `bearerToken` helper for OAuth.
14. Tests for missing bucket and malformed headers (`""`, `"abc"`, `"Basic xyz"`, `"Bearer "`).

### Phase 4 — auth architecture (P1.3, P1.4, P1.5, P1.9)
15. New `platform/auth.go`: `AuthStrategy` (`none`, `local-jwt`, `oauth`, `basic`, `local-or-oidc`), `Route.Auth`, deprecated `Route.AuthRequired` mapped to current AND semantics.
16. Refactor middleware into `authVerifier` functions returning an `authResult` (roles, token, issuer, username, local claims); `anyAuthMiddleware` for OR, `allAuthMiddleware` for legacy.
17. Fix role-less OAuth auth (verify token, then skip role matching).
18. Lazy OIDC discovery: mutex-guarded cache keyed by well-known URL/client ID/*algorithms*, use `r.Context()`, return 503 + `Retry-After` instead of panicking, retry later. Injectable verifier factory for tests.
19. Config `Auth.Server.OAuth.RolesClaim` (default `roles`); extraction supports `[]string` and object-key maps, literal key first (Zitadel URN) then dotted path. Wire `AllowedAlgorithms` into `SupportedSigningAlgs`.

### Phase 5 — config, logging, JWT (P1.6–P1.8)
20. `GetPlatformConfiguration`: `defer` unlock, extract `loadPlatformConfiguration()`, return nil on error, `errors.As` for missing-file detection, no partial config publication, real validation in `checkPlatformConfiguration`.
21. Logging: remove `init()`; back `Log` with an `atomic.Pointer[zap.Logger]` so `ConfigureLogger()` is race-free; call it from `SetPlatformConfiguration` (after unlocking) and first config load; empty level → info, unknown → `ErrInvalidLogLevel`.
22. Critical: `initializeOAuthTokenClients` and `initializeVault` currently depend on the logging `init()` having populated `internalConfig`; make both nil/error safe before removing it.
23. JWT: stop logging full tokens (`tokenPrefix` or omit); `errors.Is` for expired/signature errors; nil guard in `localJwtConfigChecks`.

### Phase 6 — request handling (P1.10, P1.11, P3.1, P3.2)
24. `ReadJsonRequestLimit` with limit + overflow → `ErrRequestBodyTooLarge`; `ReadJsonRequest` wraps with a 1 MiB default (`HTTP.Server.MaxRequestBodyBytes`); `DisallowUnknownFields`; pass `out` not `&out`; replace `ioutil`.
25. `WriteJsonResponse`: check `Write` error, no double status write.
26. Recovery + `X-Request-ID` middleware wired via `router.Use` (recovery → request ID → logging → auth → handler).

### Phase 7 — DB API (P2.1–P2.3)
27. `View`, `Update`, `Snapshot(io.Writer)`, `Integrity` (bbolt `tx.Check`), `Stats`, `SaveObjects`, `Keys`, `Scan`, `ScanPrefix`, `Count`; all fail with `ErrBoltDBNoDBObject` when unopened.

### Phase 8 — lifecycle (P2.5, P2.6)
28. `http.Server` with read-header/read/write/idle timeouts from config; `StartHttpServerWithContext` + `signal.NotifyContext` wrappers; graceful shutdown; `ErrServerClosed` not logged as error.
29. Remove the implicit `Database.BoltDb.Close()`; update `example/http-api-server` to open and close the DB itself.

### Phase 9 — cleanup, docs, CI (P2.7, P2.8, P3.3–P3.7)
30. Remove the shadowing `init()` in `database.go`; wire or delete dead config (`AllowedContentType`, `AllowedAlgorithms`); export `HTTPClientConfig`/`ClientTokenConfig` via type aliases; CORS allow-list.
31. `SavePlatformConfiguration`; `DeleteDatabase(path)` replaces `RemoveDBFile` (explicit path match).
32. README updates (optional config.yml, defaults, programmatic config, auth strategies, snapshot API, lifecycle).
33. CI workflow: `go vet`, `go test -race ./...`, `staticcheck`, plus per-example-module builds.

**Relevant files**
- `platform/boltdb.go` — mode, timeout, nil safety, hack removal, `View/Update/Snapshot/Scan`
- `platform/middleware.go` — Bearer parsing, recovery, request ID, verifier refactor, CORS
- `platform/httpserver.go` — `AuthStrategy`, route resolution, timeouts, graceful shutdown, lifecycle
- `platform/config.go` — mutex leak, validation, new fields, `SavePlatformConfiguration`
- `platform/logging.go` — `init()` removal, atomic logger, level validation
- `platform/jwt.go` — token redaction, `errors.Is`
- `platform/marshalling.go` — body limits, `io`, error handling
- `platform/database.go` — shadowed global, zero-value database
- `platform/oauth.go`, `platform/vault.go` — nil-safe init after logger change
- `example/localJwt/main.go`, `example/http-api-server/*`, `example/boltdb/*`, `readme.md` — compile fixes and docs
- `.github/workflows/ci.yml` (new)

**Verification**
1. `go vet ./...` and `go test -race ./...` pass in the root module; `go build ./...` + `go test ./...` in each example module.
2. BoltDB: new file is `0600`; reopen in-process and in a fresh process; second process hits the lock timeout within 5s; open error names path, mode, uid/gid and satisfies `errors.Is(err, fs.ErrPermission)`.
3. Auth: protected route with no roles invokes the handler; `""`/`"abc"`/`"Basic xyz"`/`"Bearer "` → 401; `local-or-oidc` accepts either token; server starts and serves other routes while the IdP is down, OIDC routes return 503.
4. Requests: handler panic → JSON 500 with request ID; 2 MiB body → 413-mappable error; every access log line carries `X-Request-ID`.
5. DB API: snapshot copy reopens and reads the same record; failed second write leaves the bucket unchanged; keys come back in lexicographic order; prefix scan filters correctly.
6. Container restart simulation on a volume as the same non-root user, no manual chmod/chown.

**Decisions**
- bbolt lands before the accessor API (differs from the doc's PR table, which would churn consumer callbacks twice).
- `AuthRequired` stays as a deprecated alias with today's AND semantics; explicit `Route.Auth` wins when set.
- `ReadAllObjects` and `RemoveDBFile` era call sites are updated rather than silently shimmed, except `ReadJsonRequest`, which keeps its signature as a thin wrapper.
- Live IdP/Vault tests become build-tagged integration tests so `go test -race ./...` is meaningful.

**Further Considerations**
1. `Snapshot`/`Scan` hold a read transaction for the whole copy; acceptable but should be documented, or optionally chunked via `tx.WriteTo` only. Recommend documented trade-off (Option A) over a copy-to-temp-file design (Option B).
2. Global state (`internalConfig`, `dbBolt`, `Log`, OIDC cache) means tests can't be parallel until it is refactored into an injectable `Platform` struct — a larger change. Recommend keeping the reset helper now and deferring the struct.
3. `example/localJwt` uses `platform.Logger`; fix to `platform.Log` in Phase 0, or add a deprecated `Logger` alias. Recommend fixing the example.
