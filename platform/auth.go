package platform

// AuthStrategy enumerates supported auth modes for a route.
type AuthStrategy string

const (
	AuthNone        AuthStrategy = "none"
	AuthLocalJWT    AuthStrategy = "local-jwt"
	AuthOAuth       AuthStrategy = "oauth"
	AuthBasic       AuthStrategy = "basic"
	AuthLocalOrOIDC AuthStrategy = "local-or-oidc"
)

func (a AuthStrategy) Enabled() bool {
	return a != "" && a != AuthNone
}
