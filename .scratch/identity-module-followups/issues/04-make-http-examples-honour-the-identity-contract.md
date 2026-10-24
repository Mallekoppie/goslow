# 04: Make the HTTP examples honour the identity contract

**What to build:** The HTTP examples demonstrate the glossary's mapping for callers that reach a handler. A verified caller lacking the required role receives 403, not 401, and receives a documented success response when it holds the role. A protected handler that somehow runs without an identity fails closed instead of reporting success.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The web-server OAuth handler returns 403 for a verified caller lacking the required role.
- [ ] The same handler returns a success response describing the caller when the role is present, rather than only logging.
- [ ] The local-JWT example's protected query handler fails closed when no identity is present; it never returns a success message.
- [ ] Both examples still build and run as documented.
