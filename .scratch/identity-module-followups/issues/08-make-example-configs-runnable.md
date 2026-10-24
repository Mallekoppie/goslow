# 08: Make example configs runnable without missing files

**What to build:** No example configuration points at a file that is not present. The Vault client certificate and key referenced by the affected configs are either shipped, the Vault section is disabled, or the documentation states that they are optional and what changes without them. The forwarding example's readme explains how roles are read from a Keycloak realm, so a valid token can pass the role gate.

**Blocked by:** 01: Land the review fixes

**Status:** ready-for-agent

- [ ] Every file referenced by an example configuration exists, or the feature that needs it is disabled and documented.
- [ ] The affected examples no longer fail a documented endpoint with a missing-file error, or the expected failure is documented.
- [ ] The forwarding example's readme explains the realm-role claim mapping needed for a valid token to pass a required role.
- [ ] The affected examples still build and boot.
