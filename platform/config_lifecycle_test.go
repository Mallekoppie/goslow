package platform

import (
	"errors"
	"sync"
	"testing"
)

func TestSetPlatformConfigurationAppliesDefaults(t *testing.T) {
	resetPlatformState(t)

	if err := SetPlatformConfiguration(Config{}); err != nil {
		t.Fatalf("SetPlatformConfiguration failed: %v", err)
	}

	cfg, err := GetPlatformConfiguration()
	if err != nil {
		t.Fatalf("GetPlatformConfiguration failed: %v", err)
	}

	if cfg.Log.Level != "info" {
		t.Fatalf("expected default log level info, got %q", cfg.Log.Level)
	}
	if cfg.Log.FilePath == "" {
		t.Fatal("expected a default log file path")
	}
	if cfg.HTTP.Server.ReadHeaderTimeoutSeconds <= 0 ||
		cfg.HTTP.Server.ReadTimeoutSeconds <= 0 ||
		cfg.HTTP.Server.WriteTimeoutSeconds <= 0 ||
		cfg.HTTP.Server.IdleTimeoutSeconds <= 0 {
		t.Fatalf("expected non-zero HTTP timeouts, got %+v", cfg.HTTP.Server)
	}
	if cfg.Grpc.Server.ReadTimeoutSeconds <= 0 || cfg.Grpc.Server.IdleTimeoutSeconds <= 0 {
		t.Fatalf("expected non-zero gRPC timeouts, got %+v", cfg.Grpc.Server)
	}
	if cfg.HTTP.Server.MaxRequestBodyBytes != DefaultReadJsonRequestLimit {
		t.Fatalf("expected body limit %d, got %d", DefaultReadJsonRequestLimit, cfg.HTTP.Server.MaxRequestBodyBytes)
	}
	if cfg.Auth.Server.OAuth.RolesClaim != "roles" {
		t.Fatalf("expected default roles claim, got %q", cfg.Auth.Server.OAuth.RolesClaim)
	}
	if cfg.Metrics.Path != "/metrics" {
		t.Fatalf("expected default metrics path, got %q", cfg.Metrics.Path)
	}
}

func TestSetPlatformConfigurationRejectsInvalidConfiguration(t *testing.T) {
	resetPlatformState(t)

	conf := createDefaultConfiguration()
	conf.Log.Level = "verbose"

	err := SetPlatformConfiguration(*conf)
	if !errors.Is(err, ErrInvalidLogLevel) {
		t.Fatalf("expected ErrInvalidLogLevel, got %v", err)
	}

	mutex.Lock()
	published := internalConfig
	mutex.Unlock()
	if published != nil {
		t.Fatal("invalid configuration must not be published")
	}
}

func TestSetPlatformConfigurationRequiresMetricsAddressWhenEnabled(t *testing.T) {
	resetPlatformState(t)

	conf := createDefaultConfiguration()
	conf.Metrics.Enabled = true

	if err := SetPlatformConfiguration(*conf); err == nil {
		t.Fatal("expected an error when metrics are enabled without a listener")
	}
}

func TestConfigureLoggerWithConfigRejectsInvalidLevel(t *testing.T) {
	conf := createDefaultConfiguration()
	conf.Log.Level = "trace"

	if err := ConfigureLoggerWithConfig(conf); !errors.Is(err, ErrInvalidLogLevel) {
		t.Fatalf("expected ErrInvalidLogLevel, got %v", err)
	}
}

func TestLoggerIsSafeBeforeConfiguration(t *testing.T) {
	unconfigured := &Logger{}
	unconfigured.Info("no-op")
	unconfigured.Error("no-op")

	var nilLogger *Logger
	nilLogger.Info("no-op")
	nilLogger.Warn("no-op")
}

func TestConfigureLoggerIsRaceFreeWithLogging(t *testing.T) {
	resetPlatformState(t)

	conf := createDefaultConfiguration()
	if err := ConfigureLoggerWithConfig(conf); err != nil {
		t.Fatalf("ConfigureLoggerWithConfig failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			Log.Info("concurrent log")
			Log.Debug("concurrent debug")
		}()
		go func() {
			defer wg.Done()
			_ = ConfigureLoggerWithConfig(conf)
		}()
	}
	wg.Wait()
}

func TestApplyRuntimeConfigurationInitializesVaultAndOAuth(t *testing.T) {
	resetPlatformState(t)

	conf := createDefaultConfiguration()
	conf.Auth.Client.OAuth = []clientTokenConfig{{ID: "no-endpoint"}}

	if err := applyRuntimeConfiguration(conf); err != nil {
		t.Fatalf("applyRuntimeConfiguration failed: %v", err)
	}
	if !vaultInitialized || vaultEnabled {
		t.Fatalf("expected Vault to be initialized and disabled, got initialized=%v enabled=%v", vaultInitialized, vaultEnabled)
	}

	oAuthStateLock.RLock()
	clients := len(oAuthClientTokenConfiguration)
	tokens := oAuthTokens
	oAuthStateLock.RUnlock()
	if clients != 1 {
		t.Fatalf("expected the endpoint-less client to remain configured, got %d", clients)
	}
	if tokens == nil {
		t.Fatal("expected the OAuth token cache to be initialized")
	}
}
