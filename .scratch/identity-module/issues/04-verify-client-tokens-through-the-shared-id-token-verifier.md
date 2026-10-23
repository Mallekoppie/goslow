# 04: Verify client tokens through the shared ID-token verifier

**What to build:** Forwarded client tokens verified by the same ID-token verifier used for route authorization, with an explicit audience policy, context cancellation, and key refresh. The hand-rolled keyset path is retired, and the forwarding example is updated and configured.

**Blocked by:** 01: Add the identity authorizer

**Status:** ready-for-agent

- [ ] Client-token verification uses the same ID-token verifier as route authorization; the hand-rolled keyset path is gone.
- [ ] Verification accepts a context, honours cancellation, and checks signature, expiry, issuer, and its explicit audience policy.
- [ ] The audience policy is explicit: route ID tokens require the configured client ID; forwarded client tokens do not, and that exception is documented.
- [ ] Provider keys are refreshed when an unknown key is presented rather than cached forever.
- [ ] The forwarding example is updated and configured; it still forwards the original client token upstream.
- [ ] Tests cover valid, expired, wrong-issuer, wrong-audience, and identity-provider-unavailable cases without a network.
