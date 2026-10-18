package platform

import (
	"errors"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

var (
	ErrLocalJwtInvalidToken               = errors.New("local JWT token is invalid")
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

func (l localJwtOrganizer) ValidateLocalJwtToken(tokenString string) (map[string]interface{}, error) {
	signingMethod, key, err := localJwtConfigChecks()
	if err != nil {
		if Log != nil {
			Log.Error("Local JWT checks failed for Validation", zap.Error(err))
		}
		return nil, err
	}
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) { return key, nil }, jwt.WithValidMethods([]string{signingMethod.Alg()}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			if Log != nil {
				Log.Error("JWT token has expired", zap.Error(err))
			}
			return nil, jwt.ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrSignatureInvalid) {
			if Log != nil {
				Log.Error("JWT token signature is invalid")
			}
			return nil, jwt.ErrSignatureInvalid
		}
		if Log != nil {
			Log.Error("Error parsing local Jwt token", zap.Error(err))
		}
		return nil, err
	}
	if !token.Valid {
		if Log != nil {
			Log.Error("Invalid JWT token", zap.String("token", redactToken(tokenString)))
		}
		return nil, ErrLocalJwtInvalidToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		if Log != nil {
			Log.Error("Token claims are not of type jwt.MapClaims", zap.String("token", redactToken(tokenString)))
		}
		return nil, ErrLocalJwtInvalidToken
	}
	if len(claims) == 0 {
		if Log != nil {
			Log.Error("Token claims are empty", zap.String("token", redactToken(tokenString)))
		}
		return nil, ErrLocalJwtInvalidToken
	}
	return claims, nil
}
