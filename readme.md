# goslow

## Platform Module

The `platform` directory provides shared infrastructure for all services in the repo. It centralizes configuration, logging, database, authentication, and utility logic.

### Key Components

- **Configuration (`config.go`, `config.yml`)**  
  Loads and validates platform-wide settings (logging, HTTP/gRPC, auth, database, vault).  
  Use `GetPlatformConfiguration()` to access settings.

- **Logging (`logging.go`)**  
  Structured logging via Uber's Zap, with file rotation and log level control.  
  Controlled by `config.yml` (`platform.log`).  
  Example: `Log.Info("message", zap.String("field", "value"))`

- **Database (`boltdb.go`, `database.go`)**  
  BoltDB integration for local key-value storage.  
  Methods for saving, reading, removing objects and buckets.  
  Controlled by `config.yml` (`platform.database.boltdb`).

- **HTTP & Connect Server (`httpserver.go`, `connectserver.go`)**  
  Helpers to start HTTP servers and a Buf Connect server that speaks Connect, gRPC, and gRPC-Web on one origin.  
  Supports h2c for plaintext native gRPC, CORS/preflight, and embedded web assets.

- **HTTP Client (`httpclient.go`)**  
  Creates HTTP clients with custom config (TLS, timeouts) from `config.yml`.

- **Authentication (`jwt.go`, `oauth.go`, `middleware.go`)**  
  - Local JWT: Issue and validate tokens for stateless auth.
  - OAuth2: Token management, auto-renewal, and validation (with Vault integration for secrets).
  - Middleware: HTTP middleware and Connect interceptors for OAuth2 and JWT authentication.

- **Vault Integration (`vault.go`)**  
  HashiCorp Vault client for secret management.  
  Supports token and certificate-based auth, configurable in `config.yml`.

- **Utilities**
  - **JSON Marshalling (`marshalling.go`)**: Read/write JSON for HTTP APIs.
  - **Middleware (`middleware.go`)**: Logging, CORS, content-type, and authentication for HTTP routes.

### Usage Patterns

- Always use platform helpers for logging, config, and database access.
- Configure all platform features in `platform/config.yml`.
- Extend authentication and middleware via provided interfaces.
- Place tests for platform features in the same directory (e.g., `boltdb_test.go`, `jwt_test.go`).

### Example

```go
import "goslow/platform"

func main() {
    config, _ := platform.GetPlatformConfiguration()
    platform.Log.Info("Starting service", zap.String("component", config.Component.ComponentName))
    // Start HTTP server
    platform.StartHttpServer(routes)
}
```

---

## Example Usage Patterns

### Token Forwarding (forwardClientToken)

Demonstrates how a client token is forwarded and validated across multiple services.

**Process:**
- Dialer gets token for client service
- Dialer calls client service
- Client service validates token
- Client service gets own token
- Client service calls server service and attaches the client token
- Server service validates token and then also validates the original client token
- Server service can now do authorization based on original client token

### gRPC-Compatible Service Setup (grpc-auth, grpc-web-server)

The platform serves RPCs with [Buf Connect](https://connectrpc.com), not grpc-go.
Connect handlers speak the Connect, gRPC, and gRPC-Web protocols, so browser
clients, native gRPC clients, and `grpcurl` all work against the same listener.

**Code generation:**
- Install tools:
  - WSL: `sudo apt install -y protobuf-compiler && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest`
  - Windows: `winget install protobuf && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest`
- Run `make gen` in the example. The generated `genconnect` package is what
  `platform.ConnectService` registers.

**Upstream note:** Connect replaces the archived `improbable-eng/grpc-web`
wrapper that used to be needed for gRPC-Web. Record any upstream replacement and
its migration path here before removing this dependency.

**Manual API Testing:**
- List services:  
  `grpcurl -insecure -import-path ./server/gen -import-path ./hello -proto ./hello/hello.proto localhost:9001 list`
- Call method:  
  `grpcurl -insecure -import-path ./server/gen -import-path ./hello -proto ./hello/hello.proto -d '{"Name":"World"}' localhost:9001 gen.HelloService.SayHello`

### HTTP API Server

Implements REST endpoints for database operations and configuration queries.

**Route Definition Example:**
```go
import (
    "net/http"
    "github.com/Mallekoppie/goslow/platform"
    "your/service/package"
)

var Routes = platform.Routes{
    platform.Route{
        Path:        "/db/write",
        Method:      http.MethodPost,
        HandlerFunc: service.WriteObject,
        SlaMs:       10,
    },
    platform.Route{
        Path:        "/db/read",
        Method:      http.MethodPost,
        HandlerFunc: service.ReadObject,
        SlaMs:       0,
    },
    platform.Route{
        Path:        "/",
        Method:      http.MethodGet,
        HandlerFunc: service.HelloWorld,
        SlaMs:       0,
    },
    platform.Route{
        Path:        "/config",
        Method:      http.MethodGet,
        HandlerFunc: service.GetConfiguration,
        SlaMs:       0,
    },
    platform.Route{
        Path:        "/all",
        Method:      http.MethodGet,
        HandlerFunc: service.ReadAll,
        SlaMs:       0,
    },
}
```

**Starting the Server:**
```go
import "github.com/Mallekoppie/goslow/platform"

func main() {
    platform.StartHttpServer(Routes)
}
```

**Example Handler:**
```go
func WriteObject(w http.ResponseWriter, r *http.Request) {
    testobject := DBTestObject{}
    err := platform.JsonMarshaller.ReadJsonRequest(r.Body, &testobject)
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        return
    }
    err = platform.Database.BoltDb.SaveObject("test", testobject.Id, testobject)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        return
    }
}
```

### Local JWT Authentication (localJwt)

Shows how to implement login and protected routes using platform JWT utilities.

**Routes:**
- `/login` (POST): Accepts username/password, returns JWT token
- `/renew` (GET): Renews token (requires authentication)
- `/query` (GET): Protected query (requires authentication)

**Example Login Handler:**
```go
func HandleLogin(w http.ResponseWriter, r *http.Request) {
    login := LoginRequest{}
    platform.JsonMarshaller.ReadJsonRequest(r.Body, &login)
    if login.Username != "test" || login.Password != "test" {
        http.Error(w, "Invalid login credentials", http.StatusUnauthorized)
        return
    }
    token, err := platform.LocalJwt.NewLocalJwtToken(map[string]interface{}{
        "username": login.Username,
        "roles":    []string{"user"},
    })
    // ...return token in response...
}
```

### Connect Hosting Example (gRPC + gRPC-Web)

Each service implements `platform.ConnectService` by mounting the generated
Connect handler. The same handler answers Connect, gRPC, and gRPC-Web calls:

```go
import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "github.com/Mallekoppie/goslow/platform"
    "your/gen/genconnect"
    "connectrpc.com/connect"
    "go.uber.org/zap"
)

type Server struct{}

func (s *Server) SayHello(ctx context.Context, req *connect.Request[gen.HelloRequest]) (*connect.Response[gen.HelloResponse], error) {
    platform.Log.Info("Received SayHello request", zap.String("name", req.Msg.Name))
    return connect.NewResponse(&gen.HelloResponse{Result: "Hello " + req.Msg.Name}), nil
}

func (s *Server) Register(mux *http.ServeMux, options ...connect.HandlerOption) {
    prefix, handler := genconnect.NewHelloServiceHandler(s, options...)
    mux.Handle(prefix, handler)
}

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    err := platform.StartConnectServerWithContext(ctx, platform.ConnectServerOptions{
        Services: []platform.ConnectService{&Server{}},
    })
    if err != nil {
        platform.Log.Error("Connect server stopped", zap.Error(err))
    }
}
```

Auth interceptors install automatically from config (local JWT and/or OAuth) for
unary and streaming RPCs, on TLS and plaintext listeners. `Grpc.Server.Auth` must
name the strategy; `Auth.Server.UnauthenticatedPaths` bypasses auth for specific
procedures. Per-procedure roles come from `Grpc.Server.ProcedureRoles`. Set
`ConnectServerOptions.DisableAuthFromConfig` to install your own interceptors instead.

Clients pick the wire protocol: `connect.WithGRPC()` for native gRPC,
`connect.WithGRPCWeb()` for browsers, or no option for the Connect protocol.

### Connect with Embedded Website Example

Serve RPCs and static assets (for example a Flutter web build) from one origin.
Non-RPC paths fall through to `WebHandler`:

```go
import (
    "context"
    "embed"
    "io/fs"
    "net/http"
    "github.com/Mallekoppie/goslow/platform"
)

//go:embed ui/*
var uiAssets embed.FS

func main() {
    config := platform.Config{}
    config.Grpc.Server.ListeningAddress = "127.0.0.1:9001"
    config.Grpc.Server.TLSCertFileName = "server.crt"
    config.Grpc.Server.TLSKeyFileName = "server.key"
    config.Grpc.Server.TLSEnabled = true
    platform.SetPlatformConfiguration(config)

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    uiFS, err := fs.Sub(uiAssets, "ui")
    if err != nil {
        platform.Log.Error("failed to strip ui assets", zap.Error(err))
        return
    }

    err = platform.StartConnectServerWithContext(ctx, platform.ConnectServerOptions{
        Services:   []platform.ConnectService{&Server{}},
        WebHandler: http.FileServer(http.FS(uiFS)),
    })
    if err != nil {
        platform.Log.Error("Connect server stopped", zap.Error(err))
    }
}
```

Preflight requests are answered with exact-origin CORS headers from
`Grpc.Server.AllowedOrigins`; wildcard origins are rejected by configuration
validation. When TLS is disabled the serving `http.Server` sets
`http.Protocols` to unencrypted HTTP/2 (h2c) plus HTTP/1.1, so native gRPC
clients can use plaintext HTTP/2. If you mount `NewConnectHandler` on your own
server, set `http.Server.Protocols` (HTTP/1.1 plus unencrypted HTTP/2) yourself.

### Combined Application Server

`StartApplicationServerWithContext` serves Connect RPCs, HTTP routes, and an SPA or
static assets on one listener:

```go
err := platform.StartApplicationServerWithContext(ctx, platform.ApplicationServerOptions{
    Services:     []platform.ConnectService{&Server{}},
    ServiceNames: []string{"gen.HelloService"},
    Routes:       routes,
    SPA:          &platform.SPAOptions{Assets: uiFS, Index: "index.html"},
})
```

RPC-shaped requests never fall through to HTTP routes or assets. The server-level
write timeout is disabled so streaming RPCs stay open; each HTTP route is instead
bounded by `http.server.writetimeoutseconds` and returns `503` when it overruns.

### Custom Listener

Applications that need to own the listener and `http.Server` can build the
handler and serve it themselves:

```go
handler, err := platform.NewConnectHandler(platform.ConnectServerOptions{
    Services:   []platform.ConnectService{&Server{}},
    WebHandler: assets,
})
if err != nil {
    log.Fatal(err)
}

server := &http.Server{Addr: ":9001", Handler: handler}
log.Fatal(server.ListenAndServe())
```

### Configuration: Code vs Config File

**Using config.yml (recommended for production):**

```go
config, err := platform.GetPlatformConfiguration()
address := config.HTTP.Server.ListeningAddress
```

**Hardcoding configuration in code (for testing/dev):**

```go
import "goslow/platform"

func main() {
    conf := platform.Config{}
    conf.Log.Level = "debug"
    conf.HTTP.Server.ListeningAddress = "127.0.0.1:9000"
    conf.Database.BoltDB.Enabled = true
    conf.Database.BoltDB.FileName = "./bolt.db"
    if err := platform.SetPlatformConfiguration(conf); err != nil {
        log.Fatal(err)
    }
    platform.StartHttpServer(routes)
}
```

`SetPlatformConfiguration` applies defaults, validates, publishes atomically, and
initializes logging, Vault, and OAuth client renewal. It returns an error for
invalid configuration (for example an unknown log level or wildcard CORS origin)
and never publishes partially valid state: if runtime initialization fails the
previously published configuration is restored and nothing new is cached.

### Configuration Defaults

`config.yml` is optional. When it is not found the platform logs a notice and uses
these defaults:

| Setting | Default |
| --- | --- |
| `log.level` | `info` |
| `http.server.listeningaddress` | `127.0.0.1:10000` |
| `grpc.server.listeningaddress` | `127.0.0.1:9001` |
| `http.server.readheadertimeoutseconds` | `5` |
| `http.server.readtimeoutseconds` | `30` |
| `http.server.writetimeoutseconds` | `60` |
| `http.server.idletimeoutseconds` | `120` |
| `database.boltdb.enabled` | `false` |
| `database.boltdb.opentimeoutseconds` | `5` |
| `auth.server.oauth.rolesclaim` | `roles` |
| `http.server.allowedorigins` | empty (CORS disabled) |
| `http.server.maxrequestbodybytes` | `1048576` |
| `grpc.server.auth` | unset (startup error if JWT/OAuth is enabled) |
| `grpc.server.streamtimeoutseconds` | `0` (unlimited) |
| `grpc.server.healthenabled` / `reflectionenabled` | `false` |
| `metrics.enabled` | `false` |

`GetPlatformConfiguration()` loads once and caches the result; `SetPlatformConfiguration()`
overrides it for tests or programmatic setups. `SavePlatformConfiguration()` writes the
current configuration back to the file viper loaded.

### Route Auth Strategies

Routes can pick an explicit strategy with `Route.Auth`:

| Strategy | Behavior |
| --- | --- |
| `none` | No authentication |
| `local-jwt` | Requires a valid platform-issued JWT |
| `oauth` | Requires a valid OIDC ID token (roles optional) |
| `basic` | Requires HTTP basic credentials |
| `local-or-oidc` | Accepts either a local JWT or an OIDC token |

```go
platform.Route{
    Path:          "/admin",
    Method:        http.MethodGet,
    Auth:          platform.AuthOAuth,
    RolesRequired: []string{"admin"},
    HandlerFunc:   adminHandler,
}
```

An explicit `Route.Auth` is mandatory: a route without one fails router
construction. Use `Auth: platform.AuthNone` for a deliberately public route.
A route that requests a disabled strategy also fails at startup.

An OAuth route with no `RolesRequired` only verifies the token. If the identity provider
cannot be reached at request time the route returns `503 Service Unavailable` with a
`Retry-After` header instead of panicking, and other routes keep serving.

Roles are read from `auth.server.oauth.rolesclaim`. The literal claim key is tried first
(handy for Zitadel URNs), then a dotted path such as `realm_access.roles`. The claim may
be a list of strings or an object whose keys are the roles.

### Unauthenticated Exclusion List

One list covers HTTP routes and RPC procedures:

```yaml
platform:
  auth:
    server:
      unauthenticatedpaths:
        - POST /auth/login          # method-qualified HTTP route
        - /gen.HelloService/Login   # RPC procedure (unary and streaming)
        - /healthz
```

Rules:

- Entries are exact paths or `METHOD /path`; wildcards are not supported.
- Excluded routes may omit `Route.Auth`; declaring a real strategy on an excluded
  route is a startup error.
- Connect procedures listed here bypass the auth interceptor.
- `/metrics` is never on the application origin, so it needs no entry.

### Local Login Flow

Local JWT login is the bootstrap call that must be public:

1. The client calls the login route or procedure, which is in the exclusion list.
2. The handler validates credentials and calls
   `platform.LocalJwt.NewLocalJwtToken(claims)`. `iat` is set automatically and
   `exp` follows `auth.server.localjwt.jwtexpiration` (minutes).
3. The client sends `Authorization: Bearer <token>` on later calls.
4. Expired or wrongly signed tokens return `401`/`CodeUnauthenticated`; there is no
   refresh token, so the client logs in again.

Forgetting the login entry makes the login endpoint itself require a token, which
is the intended fail-closed behavior.

```yaml
platform:
  auth:
    server:
      unauthenticatedpaths: ["POST /auth/login", "/gen.HelloService/Login"]
      localjwt:
        enabled: true
        jwtsigningkey: "replace-me"
        jwtsigningmethod: HS256
        jwtexpiration: 60
  grpc:
    server:
      auth: local-jwt
      procedureroles:
        /gen.HelloService/SayHello: [admin]
```

### Metrics

Metrics are always isolated on a dedicated listener and are never mounted on the
application origin. Prometheus scrapes the metrics port directly; no application
credentials are involved.

```yaml
platform:
  metrics:
    enabled: true
    listenaddress: 127.0.0.1:9100
    path: /metrics
```

```yaml
scrape_configs:
  - job_name: goslow
    static_configs:
      - targets: ["127.0.0.1:9100"]
```

Exported families: `goslow_http_requests_total`, `goslow_http_request_duration_seconds`,
`goslow_rpc_requests_total`, `goslow_rpc_request_duration_seconds`,
`goslow_auth_decisions_total` (`allowed`/`denied`/`anonymous`), and
`goslow_inflight_requests`. Labels use registered routes and procedure names only;
tokens, headers, and bodies are never recorded. The listener starts and stops with
the server context.

### Health and Reflection

`grpc.server.healthenabled` registers `grpc.health.v1.Health` for the service names
in `ConnectServerOptions.ServiceNames`; `grpc.server.reflectionenabled` registers
the v1 and v1alpha reflection handlers. Names must be fully qualified, for example
`gen.HelloService`. Enabling either without service names fails handler construction
instead of registering an empty checker. Both are disabled by default and are meant
for development and tests. With reflection enabled:

```bash
grpcurl -insecure -use-reflection localhost:9001 list
grpcurl -insecure -use-reflection localhost:9001 grpc.health.v1.Health/Check
```

### Database API

The Bolt database opens lazily on first use and closes with `platform.Database.BoltDb.Close()`.

```go
err := platform.Database.BoltDb.SaveObject("items", "item-1", item)

var stored Item
err = platform.Database.BoltDb.ReadObject("items", "item-1", &stored)

err = platform.Database.BoltDb.SaveObjects("items", map[string]interface{}{
    "item-2": second,
    "item-3": third,
})

keys, err := platform.Database.BoltDb.Keys("items")        // lexicographic
err = platform.Database.BoltDb.ScanPrefix("items", "item-", func(key, value []byte) error {
    return nil
})
count, err := platform.Database.BoltDb.Count("items")
```

Transactions are exposed through `View` and `Update`. The callback runs while the handle
lock is held, so it must not call other database methods:

```go
err := platform.Database.BoltDb.Update(func(tx *bbolt.Tx) error {
    return tx.Bucket([]byte("items")).Put([]byte("item-4"), data)
})
```

`Snapshot(io.Writer)` writes a consistent copy that can be reopened with bbolt, `Integrity()`
runs bbolt's structural check, and `Stats()` returns bbolt statistics. `DeleteDatabase(path)`
removes a specific database file and only closes the open handle when the path matches.

### HTTP Server Lifecycle

`StartHttpServer` blocks until `SIGINT`/`SIGTERM` and then shuts down gracefully. Use
`StartHttpServerWithContext(ctx, routes)` to control shutdown yourself:

```go
ctx, cancel := context.WithCancel(context.Background())
go func() {
    <-stopSignal
    cancel()
}()

if err := platform.StartHttpServerWithContext(ctx, routes); err != nil {
    log.Fatal(err)
}
```

Read-header, read, write, and idle timeouts come from `http.server.*` in the config.
The server no longer closes the database for you; applications own that lifecycle:

```go
if conf.Database.BoltDB.Enabled {
    if err := platform.SetupBoltDB(); err != nil {
        log.Fatal(err)
    }
    defer platform.Database.BoltDb.Close()
}
```

### Request Middleware

Every router built by the platform includes:

- **Request ID** – reuses an incoming `X-Request-ID` or generates one, returns it on the
  response, and stores it in the request context (`platform.ContextRequestID`).
- **Recovery** – converts a handler panic into a JSON `500` that carries the request id.
- **Access logging** – logs method, path, status, duration, and request id; SLA breaches
  are logged as warnings.

### Testing

Run the unit suite from the repository root:

```bash
make test-unit     # go test ./...
```

Additional checks used by CI:

```bash
gofmt -l .
go vet ./...
go test -race ./...
```

Tests that need a live IdP or Vault are build-tagged `integration` and excluded from the
default run:

```bash
go test -tags integration ./platform
```

The platform tests avoid shared global state by resetting configuration, database handles,
and token-renewal goroutines between tests, so each test also passes in isolation.

### Dependencies

The root module and every example module target Go 1.27.2:

- `connectrpc.com/connect` v1.21.1 (Connect, gRPC, and gRPC-Web transport),
  with `grpchealth` v1.5.0 and `grpcreflect` v1.3.1.
- `google.golang.org/protobuf` v1.36.12, `go.etcd.io/bbolt` v1.5.0,
  `github.com/coreos/go-oidc/v3` v3.21.0, and `github.com/go-jose/go-jose/v4`
  v4.1.6 for JWKS handling. The archived `github.com/lestrrat/go-jwx` dependency
  was removed.
- `github.com/spf13/viper` v1.21.0, `go.uber.org/zap` v1.28.0,
  `github.com/prometheus/client_golang` v1.25.0, `github.com/hashicorp/vault/api`
  v1.23.0, `github.com/golang-jwt/jwt/v5` v5.3.1, and
  `gopkg.in/natefinch/lumberjack.v2` v2.2.1.
- `.devcontainer/Dockerfile` pins `gopls` v0.24.0, `staticcheck` v0.8.2, `dlv`
  v1.27.2, `protoc-gen-go` v1.36.12, and `protoc-gen-connect-go` v1.21.1, all
  compatible with `GO_VERSION`; bump them together with the image. CI installs
  the same `staticcheck` v0.8.2.

Every module under `example/` carries its own `go.mod` with a `replace` to the
repository root. Run `go mod tidy` inside an example after changing root
dependencies.

