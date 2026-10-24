# 04: Verify client tokens through the shared ID-token verifier

**What to build:** Forwarded client tokens verified by the same ID-token verifier used for route authorization, with an explicit audience policy, context cancellation, and key refresh. The hand-rolled keyset path is retired, and the forwarding example is updated and configured.

**Blocked by:** 01: Add the identity authorizer

**Status:** resolved

- [x] Client-token verification uses the same ID-token verifier as route authorization; the hand-rolled keyset path is gone.
- [x] Verification accepts a context, honours cancellation, and checks signature, expiry, issuer, and its explicit audience policy.
- [x] The audience policy is explicit: route ID tokens require the configured client ID; forwarded client tokens do not, and that exception is documented.
- [x] Provider keys are refreshed when an unknown key is presented rather than cached forever.
- [x] The forwarding example is updated and configured; it still forwards the original client token upstream.
- [x] Tests cover valid, expired, wrong-issuer, wrong-audience, and identity-provider-unavailable cases without a network.

## Comments

Completed in `fc035cd`. `OAuth.ValidateClientToken` now takes a context and
delegates to the same go-oidc verifier used by route authorization, with an
explicit `acceptAnyAudience` policy because forwarded tokens were minted for
another client. The hand-rolled issuer discovery, keyset cache, and
`IDPWellKnownConfiguration` are deleted; go-oidc's shared `RemoteKeySet`
refreshes keys when an unknown key ID is presented. The forwarding example is
configured for `federate-token-server`/`federate-token-client`, keeps forwarding
the original client token upstream, and returns 503 with `Retry-After` for an
identity-provider outage. Covered by `platform/clienttoken_test.go` for valid,
expired, wrong-issuer, wrong-audience, discovery and key-fetch outages,
pre-cancelled and in-flight cancellation, key rotation, and cached reuse.
Verified with `go test ./...`, `go test -race ./...`, `go vet ./...`,
`gofmt -l .`, and `make build-examples`.
