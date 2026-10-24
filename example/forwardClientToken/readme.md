# Overview
This example shows how a client token can be forwarded to a remote API.

# Setup
Set up IDP clients for the client and server services, then update each
`config.yml` with the realm URL and client IDs:
- `client/config.yml` authorizes callers issued to `federate-token-client` and
  requests its own token as `federate-token-server`.
- `server/config.yml` authorizes callers issued to `federate-token-server`.

# Process
* Dialer gets token for client service
* Dialer calls client service
* Client service validates token
* Client service gets own token
* Client service calls server service and attaches the client token
* Server service validates token and then also validates the original client token
* Server service can now do authorization based on original client token

# Audience policy
The server's own route token must be issued to its configured client ID
(`federate-token-server`). The forwarded client token was issued to a different
client (`federate-token-client`), so `ValidateClientToken` verifies its
signature, expiry, and issuer but deliberately skips the audience check.
Requiring the server's client ID there would reject every forwarded token.
