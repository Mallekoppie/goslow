package platform

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
)

func TestGetComponentConfiguration(t *testing.T) {
	resetPlatformState(t)

	type componentConfig struct {
		Name  string
		Limit int
	}

	viper.Set("demo", map[string]interface{}{"name": "example", "limit": 3})

	var conf componentConfig
	if err := GetComponentConfiguration("demo", &conf); err != nil {
		t.Fatalf("GetComponentConfiguration failed: %v", err)
	}
	if conf.Name != "example" || conf.Limit != 3 {
		t.Fatalf("expected decoded values, got %+v", conf)
	}

	if err := GetComponentConfiguration("demo", nil); err == nil {
		t.Fatal("expected an error for a nil target")
	}
}

func TestSavePlatformConfigurationWithoutLoadedFile(t *testing.T) {
	resetPlatformState(t)

	if err := SavePlatformConfiguration(*createDefaultConfiguration()); err == nil {
		t.Fatal("expected an error when no config file is loaded")
	}
}

func TestRedactToken(t *testing.T) {
	if got := redactToken("short"); got != "[redacted]" {
		t.Fatalf("expected short tokens to be fully redacted, got %q", got)
	}

	long := "abcdefghijklmnop"
	if got, want := redactToken(long), "abcdefgh..."; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLocalJwtConfigErrorPaths(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(*Config)
		want      error
	}{
		{
			name: "disabled",
			configure: func(c *Config) {
				c.Auth.Server.LocalJwt.Enabled = false
			},
			want: ErrLocalJwtNotEnabled,
		},
		{
			name: "missing signing key",
			configure: func(c *Config) {
				c.Auth.Server.LocalJwt.Enabled = true
				c.Auth.Server.LocalJwt.JwtSigningMethod = "HS256"
			},
			want: ErrLocalJwtSigningKeyNotConfigured,
		},
		{
			name: "invalid signing method",
			configure: func(c *Config) {
				c.Auth.Server.LocalJwt.Enabled = true
				c.Auth.Server.LocalJwt.JwtSigningKey = "key"
				c.Auth.Server.LocalJwt.JwtSigningMethod = "RS256"
			},
			want: ErrLocalJwtSigningMethodNotConfigured,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetPlatformState(t)
			cfg := createDefaultConfiguration()
			testCase.configure(cfg)
			SetPlatformConfiguration(*cfg)

			if _, err := LocalJwt.NewLocalJwtToken(map[string]interface{}{"sub": "a"}); !errors.Is(err, testCase.want) {
				t.Fatalf("expected %v, got %v", testCase.want, err)
			}
		})
	}
}

func TestGetExpiryTimeFromToken(t *testing.T) {
	if got := getExpiryTimeFromToken("opaque-token"); !got.IsZero() {
		t.Fatalf("expected a zero time for opaque tokens, got %s", got)
	}

	expiry := time.Now().Add(time.Hour).Truncate(time.Second)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"exp": expiry.Unix()})
	signed, err := token.SignedString([]byte("key"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if got := getExpiryTimeFromToken(signed); !got.Equal(expiry) {
		t.Fatalf("expected %s, got %s", expiry, got)
	}
}

func TestProcessVaultSecretResponse(t *testing.T) {
	secrets := processVaultSecretResponse(map[string]interface{}{
		"data": map[string]interface{}{"username": "alice", "password": "s3cret"},
	})
	if secrets["username"] != "alice" || secrets["password"] != "s3cret" {
		t.Fatalf("unexpected secrets: %v", secrets)
	}

	empty := processVaultSecretResponse(map[string]interface{}{})
	if len(empty) != 0 {
		t.Fatalf("expected no secrets, got %v", empty)
	}
}

func TestVaultGetSecretsWhenDisabled(t *testing.T) {
	resetPlatformState(t)

	secrets, err := Vault.GetSecrets("kv/data/demo")
	if !errors.Is(err, ErrVaultNotEnabled) {
		t.Fatalf("expected ErrVaultNotEnabled, got %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected no secrets, got %v", secrets)
	}
}

func TestCreateVaultClientAppliesToken(t *testing.T) {
	cfg := createDefaultConfiguration()
	cfg.Vault.TimeoutSeconds = 1
	cfg.Vault.Token.Enabled = true
	cfg.Vault.Token.Token = "root-token"

	client, err := createVaultClient("http://127.0.0.1:8200", cfg)
	if err != nil {
		t.Fatalf("createVaultClient failed: %v", err)
	}
	if client.Token() != "root-token" {
		t.Fatalf("expected the configured token, got %q", client.Token())
	}
}

func TestOAuthGetTokenWithoutConfig(t *testing.T) {
	resetPlatformState(t)

	if _, err := OAuth.GetToken("missing"); err == nil {
		t.Fatal("expected an error when no oauth client config exists")
	}
}

func TestInitializeLoggerKeepsExistingLogger(t *testing.T) {
	resetPlatformState(t)
	Log.Info("still usable")

	existing := Log
	InitializeLogger()
	if Log != existing {
		t.Fatal("InitializeLogger must not replace an already configured logger")
	}
}

func TestCreateOAuthHTTPClient(t *testing.T) {
	client := createOAuthHTTPClient()
	if client == nil {
		t.Fatal("expected an http client")
	}
	if client.Timeout != time.Duration(oAuthRequestTimeout)*time.Second {
		t.Fatalf("unexpected timeout: %s", client.Timeout)
	}
}
