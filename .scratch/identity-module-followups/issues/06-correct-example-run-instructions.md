# 06: Correct the existing example run instructions and configs

**What to build:** Following each existing example readme from a clean checkout works. The boltdb readme no longer references a config file that is absent or an initializer that does not exist. The grpc-auth example has one source of truth for its local JWT settings and a readme with complete run steps and the Connect code-generation prerequisite. The grpc-web readme stops claiming reflection that the server never enables, or the server enables it and the readme documents it.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] The boltdb readme only describes files, commands, and functions that exist.
- [x] The grpc-auth config and its programmatic configuration agree, or the unused config is removed.
- [x] The grpc-auth readme covers code generation, build, run, and a successful call, including the prerequisite the root readme already names.
- [x] The grpc-web readme's verification instructions match the features the server enables.
- [x] Each documented command has been executed successfully.

## Comments

Source commit `ac062d6a7d701380d19f9e2f66dc39fa2d8f7393` (`ticket/06-correct-example-run-instructions`), merged into `identity-module-followups` as merge commit `fdf1bd8e566427c66dac01215bc26dc4a817006b`.

Verification on the integration branch after the merge:

- `go build ./...` — passed
- `go vet ./...` — passed
- `gofmt -l .` — no output (clean)
- `go test ./...` — passed
- `make build-examples` — passed, building all seven examples

The grpc-auth `server/config.yml` is removed and `server/main.go` is the single source of programmatic settings; the readmes for boltdb, grpc-auth, and grpc-web now describe only files and commands that exist.
