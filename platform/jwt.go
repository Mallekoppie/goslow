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

	LocalJwt localJwtOrganizer
)

type localJwtOrganizer struct{}

func localJwtConfigChecks() (jwt.SigningMethod, []byte, error) {
	if !internalConfig.Auth.Server.LocalJwt.Enabled {
		Log.Error("Local JWT authentication is not enabled in the configuration")
		return nil, nil, ErrLocalJwtNotEnabled
	}

	if len(internalConfig.Auth.Server.LocalJwt.JwtSigningKey) == 0 {
		Log.Error("JWT signing key is not configured")
		return nil, nil, ErrLocalJwtSigningKeyNotConfigured
	}

	if len(internalConfig.Auth.Server.LocalJwt.JwtSigningMethod) == 0 || (internalConfig.Auth.Server.LocalJwt.JwtSigningMethod != "HS256" &&
		internalConfig.Auth.Server.LocalJwt.JwtSigningMethod != "HS384" &&
		internalConfig.Auth.Server.LocalJwt.JwtSigningMethod != "HS512") {
		Log.Error("JWT signing method is not configured or invalid")
		return nil, nil, ErrLocalJwtSigningMethodNotConfigured
	}

	var signingMethod jwt.SigningMethod
	switch internalConfig.Auth.Server.LocalJwt.JwtSigningMethod {
	case "HS256":
		signingMethod = jwt.SigningMethodHS256
	case "HS384":
		signingMethod = jwt.SigningMethodHS384
	case "HS512":
		signingMethod = jwt.SigningMethodHS512
	}

	key := []byte(internalConfig.Auth.Server.LocalJwt.JwtSigningKey)

	return signingMethod, key, nil
}

func (l localJwtOrganizer) NewLocalJwtToken(claims map[string](interface{})) (string, error) {
	signingMethod, key, err := localJwtConfigChecks()
	if err != nil {
		Log.Error("Local JWT checks failed for token creation", zap.Error(err))
		return "", err
	}

	claimsForToken := jwt.MapClaims{}
	for key, value := range claims {
		claimsForToken[key] = value
	}

	// Add standard claims
	claimsForToken["iat"] = time.Now().Unix() // Issued at time
	if internalConfig.Auth.Server.LocalJwt.JwtExpiration > 0 {
		claimsForToken["exp"] = time.Now().Add(time.Duration(internalConfig.Auth.Server.LocalJwt.JwtExpiration) * time.Minute).Unix()
	}

	token := jwt.NewWithClaims(signingMethod, claimsForToken)
	signedToken, err := token.SignedString(key)
	if err != nil {
		Log.Error("Error signing token", zap.Error(err))
		return "", err
	}

	return signedToken, nil
}

func (l localJwtOrganizer) ValidateLocalJwtToken(tokenString string) (map[string]interface{}, error) {
	signingMethod, key, err := localJwtConfigChecks()
	if err != nil {
		Log.Error("Local JWT checks failed for Validation", zap.Error(err))
		return nil, err
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return key, nil
	}, jwt.WithValidMethods([]string{signingMethod.Alg()}))
	if err != nil {
		if err == jwt.ErrTokenExpired {
			Log.Error("JWT token has expired", zap.Error(err))
			return nil, jwt.ErrTokenExpired
		} else if err == jwt.ErrSignatureInvalid {
			Log.Error("JWT token signature is invalid")
			return nil, jwt.ErrSignatureInvalid
		}
		Log.Error("Error parsing local Jwt token", zap.Error(err))
		return nil, err
	}

	if !token.Valid {
		Log.Error("Invalid JWT token", zap.String("token", tokenString))
		return nil, ErrLocalJwtInvalidToken
	}

	if claims, ok := token.Claims.(jwt.MapClaims); !ok {
		Log.Error("Token claims are not of type jwt.MapClaims", zap.String("token", tokenString))
		return nil, ErrLocalJwtInvalidToken
	} else if len(claims) == 0 {
		Log.Error("Token claims are empty", zap.String("token", tokenString))
		return nil, ErrLocalJwtInvalidToken
	} else {
		return claims, nil
	}
}
