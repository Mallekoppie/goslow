# 08: Make example configs runnable without missing files

**What to build:** No example configuration points at a file that is not present. The Vault client certificate and key referenced by the affected configs are either shipped, the Vault section is disabled, or the documentation states that they are optional and what changes without them. The forwarding example's readme explains how roles are read from a Keycloak realm, so a valid token can pass the role gate.

**Blocked by:** 01: Land the review fixes

**Status:** resolved

- [x] Every file referenced by an example configuration exists, or the feature that needs it is disabled and documented.
- [x] The affected examples no longer fail a documented endpoint with a missing-file error, or the expected failure is documented.
- [x] The forwarding example's readme explains the realm-role claim mapping needed for a valid token to pass a required role.
- [x] The affected examples still build and boot.

## Comments

Source commit: `db9c10b16e02d29799fc953379d569fe9201b375` (`ticket/08-runnable-example-configs`).
Merge commit: `a560175aeee694a1ba00b51298b113171e52f59c` (`Merge ticket 08: make example configs runnable without missing files`).

The merge into `identity-module-followups` was conflict-free, including against the tickets 06 and 09 changes merged after the source branch last synced (`b331ec2`).

Verification performed from `/workspaces/goslow` on the merge commit:

- `go build ./...` - passed
- `go vet ./...` - passed
- `gofmt -l .` - clean (no files listed)
- `go test -count=1 ./...` - passed (`platform` 1.860s; remaining packages have no test files)
- `make build-examples` - passed (boltdb, grpc-auth client/server, grpc-web-server client/server, http-web-server, localJwt)
