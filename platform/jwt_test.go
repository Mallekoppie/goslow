package platform

import "testing"

func configurePlatformForLocalJwtTests(t *testing.T) {
	t.Helper()
	resetPlatformState(t)

	cfg := createDefaultConfiguration()
	cfg.Auth.Server.LocalJwt.Enabled = true
	cfg.Auth.Server.LocalJwt.JwtSigningKey = "test"
	cfg.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
	SetPlatformConfiguration(*cfg)
}

// Test to create a JWT token
func TestCreateJWTToken(t *testing.T) {
	configurePlatformForLocalJwtTests(t)
	claims := map[string]interface{}{
		"sub": "testuser",
		"aud": "testaudience",
		"iss": "testissuer",
	}

	token, err := LocalJwt.NewLocalJwtToken(claims)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	if token == "" {
		t.Fatal("Token should not be empty")
	}

	t.Logf("Generated JWT Token: %s", token)
}

// Test to validate a JWT token
func TestValidateJWTToken(t *testing.T) {
	configurePlatformForLocalJwtTests(t)

	claims := map[string]interface{}{
		"sub": "testuser",
		"aud": "testaudience",
		"iss": "testissuer",
	}

	token, err := LocalJwt.NewLocalJwtToken(claims)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	validatedClaims, err := LocalJwt.ValidateLocalJwtToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	if validatedClaims["sub"] != claims["sub"] {
		t.Errorf("Expected subject %s, got %s", claims["sub"], validatedClaims["sub"])
	}
}
