# 07: Add run readmes for the HTTP examples

**What to build:** The web-server, local-JWT, and HTTP API examples each have a readme that takes a client from a clean checkout to a working request. Each covers prerequisites, the configuration it reads, the run command, its routes, and the expected authentication responses, including any placeholder identity-provider setting and the role required by the protected route.

**Blocked by:** 04: Make the HTTP examples honour the identity contract; 05: Reject discovery-document URLs in the OAuth issuer setting

**Status:** ready-for-agent

- [ ] Each of the three examples has a readme with prerequisites, configuration location, run command, route list, and expected auth outcomes.
- [ ] The web-server readme labels the placeholder issuer base and explains what to replace it with; following it against a real provider yields a working OAuth route.
- [ ] The local-JWT readme demonstrates login, then an authenticated request with the returned token.
- [ ] The HTTP API readme states that it has no protected routes.
