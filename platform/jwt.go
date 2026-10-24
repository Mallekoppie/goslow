package platform

import (
	"errors"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

var (
	ErrLocalJwtNotEnabled                 = errors.New("local JWT authentication is not enabled in the configuration")
	ErrLocalJwtSigningKeyNotConfigured    = errors.New("JWT signing key is not configured")
	ErrLocalJwtSigningMethodNotConfigured = errors.New("JWT signing method is not configured or invalid")
	LocalJwt                              localJwtOrganizer
)

type localJwtOrganizer struct{}

func localJwtConfigChecks() (jwt.SigningMethod, []byte, error) {
	cfg, err := GetPlatformConfiguration()
	if err != nil || cfg == nil {
		return nil, nil, err
	}
	if !cfg.Auth.Server.LocalJwt.Enabled {
		if Log != nil {
			Log.Error("Local JWT authentication is not enabled in the configuration")
		}
		return nil, nil, ErrLocalJwtNotEnabled
	}
	if len(cfg.Auth.Server.LocalJwt.JwtSigningKey) == 0 {
		if Log != nil {
			Log.Error("JWT signing key is not configured")
		}
		return nil, nil, ErrLocalJwtSigningKeyNotConfigured
	}
	if len(cfg.Auth.Server.LocalJwt.JwtSigningMethod) == 0 || (cfg.Auth.Server.LocalJwt.JwtSigningMethod != "HS256" && cfg.Auth.Server.LocalJwt.JwtSigningMethod != "HS384" && cfg.Auth.Server.LocalJwt.JwtSigningMethod != "HS512") {
		if Log != nil {
			Log.Error("JWT signing method is not configured or invalid")
		}
		return nil, nil, ErrLocalJwtSigningMethodNotConfigured
	}
	var signingMethod jwt.SigningMethod
	switch cfg.Auth.Server.LocalJwt.JwtSigningMethod {
	case "HS256":
		signingMethod = jwt.SigningMethodHS256
	case "HS384":
		signingMethod = jwt.SigningMethodHS384
	case "HS512":
		signingMethod = jwt.SigningMethodHS512
	}
	return signingMethod, []byte(cfg.Auth.Server.LocalJwt.JwtSigningKey), nil
}

func redactToken(token string) string {
	if len(token) <= 8 {
		return "[redacted]"
	}
	return token[:8] + "..."
}

// RedactToken returns a log-safe token prefix. Use it instead of logging raw
// credentials.
func RedactToken(token string) string {
	return redactToken(token)
}

func (l localJwtOrganizer) NewLocalJwtToken(claims map[string]interface{}) (string, error) {
	signingMethod, key, err := localJwtConfigChecks()
	if err != nil {
		if Log != nil {
			Log.Error("Local JWT checks failed for token creation", zap.Error(err))
		}
		return "", err
	}
	claimsForToken := jwt.MapClaims{}
	for k, v := range claims {
		claimsForToken[k] = v
	}
	claimsForToken["iat"] = time.Now().Unix()
	cfg, _ := GetPlatformConfiguration()
	if cfg != nil && cfg.Auth.Server.LocalJwt.JwtExpiration > 0 {
		claimsForToken["exp"] = time.Now().Add(time.Duration(cfg.Auth.Server.LocalJwt.JwtExpiration) * time.Minute).Unix()
	}
	token := jwt.NewWithClaims(signingMethod, claimsForToken)
	signedToken, err := token.SignedString(key)
	if err != nil {
		if Log != nil {
			Log.Error("Error signing token", zap.Error(err))
		}
		return "", err
	}
	return signedToken, nil
}
