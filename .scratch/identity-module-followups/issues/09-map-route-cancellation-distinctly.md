# 09: Map route-path cancellation distinctly

**What to build:** A request cancelled while route credentials are being verified is reported as cancellation, not as a rejected credential or a provider outage. Adapters can tell cancellation apart from the three authorization outcomes, and metrics do not record it as a denial. Forwarded client-token verification behaviour is unchanged.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Cancellation during verifier construction or token verification does not produce authentication-required or unavailable.
- [ ] Non-cancellation verification failures keep their existing categories.
- [ ] Metrics record a cancelled attempt distinctly from a denied one.
- [ ] Tests cover cancellation before and during a key fetch on the route path.
