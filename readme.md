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

- **HTTP & gRPC Server (`httpserver.go`, `grpcserver.go`)**  
  Helpers to start HTTP/gRPC servers with middleware for logging, CORS, and authentication.  
  Supports serving static web assets.

- **HTTP Client (`httpclient.go`)**  
  Creates HTTP clients with custom config (TLS, timeouts) from `config.yml`.

- **Authentication (`jwt.go`, `oauth.go`, `middleware.go`)**  
  - Local JWT: Issue and validate tokens for stateless auth.
  - OAuth2: Token management, auto-renewal, and validation (with Vault integration for secrets).
  - Middleware: HTTP/gRPC interceptors for Basic, OAuth2, and JWT authentication.

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

### gRPC Service Setup (grpc, grpc-auth, grpc-web-server)

**Protobuf Setup:**
- Install tools:
  - WSL: `sudo apt install -y protobuf-compiler && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`
  - Windows: `winget install protobuf && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`
- Use provided Makefile for code generation.

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

### gRPC Hosting Example

Start a gRPC server with one or more services. Each service must implement the `Register` method:

```go
import (
    "context"
    "google.golang.org/grpc"
    "github.com/Mallekoppie/goslow/platform"
    "your/gen/package"
)

type Server struct {
    gen.UnimplementedHelloServiceServer
}

// Implement Register to attach your service to the gRPC server
func (s *Server) Register(server *grpc.Server) {
    gen.RegisterHelloServiceServer(server, s)
}

func (s *Server) SayHello(ctx context.Context, req *gen.HelloRequest) (*gen.HelloResponse, error) {
    platform.Log.Info("Received SayHello request", zap.String("name", req.Name))
    return &gen.HelloResponse{Result: "Hello " + req.Name}, nil
}

func main() {
    services := []platform.GRPCService{
        &Server{},
    }
    platform.StartGrpcServer(services)
}
```

### gRPC with Embedded Website Example

Serve both gRPC and static web assets from the same server. Each service must implement the `Register` method:

```go
import (
    "context"
    "embed"
    "google.golang.org/grpc"
    "github.com/Mallekoppie/goslow/platform"
    "your/gen/package"
)

//go:embed ui/*
var uiAssets embed.FS

type Server struct {
    gen.UnimplementedHelloServiceServer
}

func (s *Server) Register(server *grpc.Server) {
    gen.RegisterHelloServiceServer(server, s)
}

func (s *Server) SayHello(ctx context.Context, req *gen.HelloRequest) (*gen.HelloResponse, error) {
    platform.Log.Info("Received SayHello request", zap.String("name", req.Name))
    return &gen.HelloResponse{Result: "Hello " + req.Name}, nil
}

func main() {
    config := platform.Config{}
    config.Component.ComponentName = "grpc-web"
    config.Grpc.Server.ListeningAddress = "127.0.0.1:9001"
    config.Grpc.Server.TLSCertFileName = "server.crt"
    config.Grpc.Server.TLSKeyFileName = "server.key"
    config.Grpc.Server.TLSEnabled = true
    platform.SetPlatformConfiguration(config)

    services := []platform.GRPCService{
        &Server{},
    }
    platform.StartGrpcServerWithWeb(services, "ui", &uiAssets)
}
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
    platform.SetPlatformConfiguration(conf)
    platform.StartHttpServer(routes)
}
```

### Configuration Defaults

`config.yml` is optional. When it is not found the platform logs a notice and uses
these defaults:

| Setting | Default |
| --- | --- |
| `log.level` | `info` |
| `http.server.listeningaddress` | `127.0.0.1:10000` |
| `http.server.readheadertimeoutseconds` | `5` |
| `http.server.readtimeoutseconds` | `30` |
| `http.server.writetimeoutseconds` | `60` |
| `http.server.idletimeoutseconds` | `120` |
| `database.boltdb.enabled` | `false` |
| `database.boltdb.opentimeoutseconds` | `5` |
| `auth.server.oauth.rolesclaim` | `roles` |

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

An explicit `Route.Auth` wins. The legacy `Route.AuthRequired: true` keeps its previous
AND semantics: every server auth strategy that is enabled must accept the request.

An OAuth route with no `RolesRequired` only verifies the token. If the identity provider
cannot be reached at request time the route returns `503 Service Unavailable` with a
`Retry-After` header instead of panicking, and other routes keep serving.

Roles are read from `auth.server.oauth.rolesclaim`. The literal claim key is tried first
(handy for Zitadel URNs), then a dotted path such as `realm_access.roles`. The claim may
be a list of strings or an object whose keys are the roles.

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
`RemoveDBFile()` remains as a deprecated wrapper around the configured path.

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

