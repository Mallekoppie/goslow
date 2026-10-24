# 05: Reject discovery-document URLs in the OAuth issuer setting

**What to build:** Configuring the OAuth issuer with a full well-known discovery URL fails startup validation with a message telling the operator to supply the issuer base, because the platform appends the discovery path itself. A correctly shaped issuer base still starts normally, and the HTTP example's configuration passes validation.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Serving configuration validation rejects a value that already contains the discovery-document path, with an error naming the issuer base as the expected form.
- [ ] A valid issuer base URL remains accepted and the other OAuth validations are unchanged.
- [ ] Tests cover both the rejected and accepted forms.
- [ ] The HTTP example's checked-in configuration passes validation.
