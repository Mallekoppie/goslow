# 09: Map route-path cancellation distinctly

**What to build:** A request cancelled while route credentials are being verified is reported as cancellation, not as a rejected credential or a provider outage. Adapters can tell cancellation apart from the three authorization outcomes, and metrics do not record it as a denial. Forwarded client-token verification behaviour is unchanged.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] Cancellation during verifier construction or token verification does not produce authentication-required or unavailable.
- [x] Non-cancellation verification failures keep their existing categories.
- [x] Metrics record a cancelled attempt distinctly from a denied one.
- [x] Tests cover cancellation before and during a key fetch on the route path.

## Comments

Resolved by merging `ticket/09-map-route-cancellation` into `identity-module-followups`.

- Source branch tip: `c4e500d` (feature commit `cdfd1d3` "Map route-path cancellation distinctly").
- Merge commit: `1d11457` ("Merge ticket 09: map route-path cancellation distinctly"), no conflicts.
- Verification performed from `/workspaces/goslow` after the merge: `go build ./...` passed, `go vet ./...` passed, `gofmt -l .` reported no files, `go test -count=1 ./...` passed, and `go test -race -count=1 ./...` passed.
- Implementer's note: an authenticated HTTP request cancelled by its caller is reported with status 499 ("Client Closed Request"). This is a non-standard convention (not an IANA-registered status code). gRPC maps the same cancellation to `CodeCanceled`, and auth metrics record a `cancelled` outcome distinctly from `denied`.
