# 05: Reject discovery-document URLs in the OAuth issuer setting

**What to build:** Configuring the OAuth issuer with a full well-known discovery URL fails startup validation with a message telling the operator to supply the issuer base, because the platform appends the discovery path itself. A correctly shaped issuer base still starts normally, and the HTTP example's configuration passes validation.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] Serving configuration validation rejects a value that already contains the discovery-document path, with an error naming the issuer base as the expected form.
- [x] A valid issuer base URL remains accepted and the other OAuth validations are unchanged.
- [x] Tests cover both the rejected and accepted forms.
- [x] The HTTP example's checked-in configuration passes validation.

## Comments

- Source commit: `974495f` (branch `ticket/05-reject-discovery-issuer-urls`, tip `5cd0ec7`).
- Merge commit: `587fdb3` (`Merge ticket 05: reject discovery-document issuer URLs`).
- Verification performed from `/workspaces/goslow` after the merge: `go build ./...` (pass), `go vet ./...` (pass), `gofmt -l .` (no output), `go test ./...` (pass, `platform` 1.967s), `go test -race ./...` (pass, `platform` 3.438s). Merge completed with no conflicts.
