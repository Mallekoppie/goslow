# 06: Correct the existing example run instructions and configs

**What to build:** Following each existing example readme from a clean checkout works. The boltdb readme no longer references a config file that is absent or an initializer that does not exist. The grpc-auth example has one source of truth for its local JWT settings and a readme with complete run steps and the Connect code-generation prerequisite. The grpc-web readme stops claiming reflection that the server never enables, or the server enables it and the readme documents it.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The boltdb readme only describes files, commands, and functions that exist.
- [ ] The grpc-auth config and its programmatic configuration agree, or the unused config is removed.
- [ ] The grpc-auth readme covers code generation, build, run, and a successful call, including the prerequisite the root readme already names.
- [ ] The grpc-web readme's verification instructions match the features the server enables.
- [ ] Each documented command has been executed successfully.
