# 10: Remove the remaining auth-adapter duplication

**What to build:** The HTTP and RPC adapters classify authorization failures once instead of maintaining two matching switches. The retry delay advertised for an unavailable identity provider is defined once, and the ID-token verifier is built from an OAuth settings value rather than a fabricated partial configuration. Behaviour and transport mappings are unchanged.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] One classification produces a transport-neutral outcome; each adapter maps that outcome to its status or code.
- [ ] The retry delay is defined in one place and used by every adapter and example that advertises it.
- [ ] No partial configuration is constructed solely to carry OAuth settings to the verifier.
- [ ] Existing tests pass without changing the observable 401/403/503 and unauthenticated/permission-denied/unavailable mappings.
