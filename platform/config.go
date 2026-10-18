package platform

import (
	"errors"
	"fmt"
	"sync"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	internalConfig           *Config
	mutex                    sync.Mutex
	bootstrapLock            sync.Mutex
	ErrInvalidConfigFilePath = errors.New("invalid config file path for settings platform.log.logfilepath")
)

func writePlatformConfiguration(conf Config) error {
	applyDefaults(&conf)
	if err := validateConfiguration(&conf); err != nil {
		return err
	}
	viper.Set("platform", conf)
	return viper.WriteConfig()
}

// SavePlatformConfiguration writes the supplied configuration to the file viper
// loaded. It returns the underlying viper error when no config file is in use.
func SavePlatformConfiguration(conf Config) error {
	return writePlatformConfiguration(conf)
}

// SetPlatformConfiguration validates programmatic configuration, publishes it,
// and initializes logger, Vault, and OAuth client renewal. Invalid configuration
// is never published.
func SetPlatformConfiguration(conf Config) error {
	cfg := conf
	applyDefaults(&cfg)
	if err := validateConfiguration(&cfg); err != nil {
		return err
	}

	publishConfiguration(&cfg)
	return applyRuntimeConfiguration(&cfg)
}

func publishConfiguration(cfg *Config) {
	mutex.Lock()
	internalConfig = cfg
	mutex.Unlock()
}

func GetPlatformConfiguration() (*Config, error) {
	mutex.Lock()
	cached := internalConfig
	mutex.Unlock()
	if cached != nil {
		return cached, nil
	}

	// Serialize the first load so runtime initialization runs once.
	bootstrapLock.Lock()
	defer bootstrapLock.Unlock()

	mutex.Lock()
	cached = internalConfig
	mutex.Unlock()
	if cached != nil {
		return cached, nil
	}

	cfg, err := loadPlatformConfiguration()
	if err != nil {
		return nil, err
	}

	publishConfiguration(cfg)
	if err := applyRuntimeConfiguration(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func loadPlatformConfiguration() (*Config, error) {
	cfg := &Config{}
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./platform")
	viper.SetConfigName("config")

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			fmt.Println("config file `config.yml` not found, using default configurations")
			cfg = createDefaultConfiguration()
		} else {
			return nil, fmt.Errorf("unable to read config file: %w", err)
		}
	} else if err := viper.UnmarshalKey("platform", cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	applyDefaults(cfg)
	if err := validateConfiguration(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

// applyRuntimeConfiguration initializes subsystems that depend on configuration.
// It must be called without the configuration mutex held.
func applyRuntimeConfiguration(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if err := ConfigureLoggerWithConfig(cfg); err != nil {
		return err
	}
	if err := initializeVault(cfg); err != nil {
		return err
	}
	initializeMetrics(cfg)
	return initializeOAuthTokenClients(cfg)
}

func GetComponentConfiguration(key string, object interface{}) error {
	if object == nil {
		return errors.New("nil component configuration target")
	}
	err := viper.UnmarshalKey(key, object)
	if err != nil {
		if Log != nil {
			Log.Error("Unable to read component configuration", zap.String("configkey", key), zap.Error(err))
		}
		return err
	}
	return nil
}

type Config struct {
	Log struct {
		Level              string
		FileLoggingEnabled bool
		FilePath           string
		MaxSize            int
		MaxBackups         int
		MaxAge             int
	}

	HTTP struct {
		Server struct {
			ListeningAddress             string
			TLSCertFileName              string
			TLSKeyFileName               string
			TLSEnabled                   bool
			AllowCorsForLocalDevelopment bool
			AllowedOrigins               []string
			MaxRequestBodyBytes          int64
			ReadHeaderTimeoutSeconds     int
			ReadTimeoutSeconds           int
			WriteTimeoutSeconds          int
			IdleTimeoutSeconds           int
		}
		Clients []HTTPClientConfig
	}

	Grpc struct {
		Server struct {
			ListeningAddress         string
			TLSCertFileName          string
			TLSKeyFileName           string
			TLSEnabled               bool
			UnAuthenticatedPaths     []string
			Auth                     AuthStrategy
			ProcedureRoles           map[string][]string
			HealthEnabled            bool
			ReflectionEnabled        bool
			SlaMs                    int64
			AllowedOrigins           []string
			ReadHeaderTimeoutSeconds int
			ReadTimeoutSeconds       int
			WriteTimeoutSeconds      int
			StreamTimeoutSeconds     int
			IdleTimeoutSeconds       int
			MaxReceiveMessageBytes   int
		}
	}

	Metrics struct {
		Enabled       bool
		ListenAddress string
		Path          string
	}

	Auth struct {
		Server struct {
			UnauthenticatedPaths []string
			OAuth                struct {
				Enabled           bool
				IdpWellKnownURL   string
				ClientID          string
				AllowedAlgorithms []string
				RolesClaim        string
			}
			LocalJwt struct {
				Enabled          bool
				JwtSigningKey    string
				JwtSigningMethod string
				JwtExpiration    int64
			}
			Basic struct {
				Enabled      bool
				AllowedUsers map[string]string
			}
		}
		Client struct {
			OAuth []clientTokenConfig
		}
	}

	Component struct {
		ComponentName string
	}

	Database struct {
		BoltDB struct {
			Enabled            bool
			FileName           string
			OpenTimeoutSeconds int64
		}
	}

	Vault struct {
		Enabled            bool
		AddressList        []string
		IsLocalAgent       bool
		InsecureSkipVerify bool
		CaCert             string
		TimeoutSeconds     int64
		MaxRetries         int
		Token              struct {
			Enabled   bool
			TokenPath string
			Token     string
		}
		Cert struct {
			Enabled  bool
			CertFile string
			KeyFile  string
		}
	}
}

type httpClientConfig struct {
	ID                 string
	TLSVerify          bool
	MaxIdleConnections int
	RequestTimeout     int
}

// HTTPClientConfig is the exported alias for the per-client HTTP configuration.
type HTTPClientConfig = httpClientConfig

// ClientTokenConfig is the exported alias for the OAuth client token configuration.
type ClientTokenConfig = clientTokenConfig

type clientTokenConfig struct {
	ID                         string
	IdpWellKnownURL            string
	RenewCheckIntervalSeconds  float64
	RenewCheckTimeMinutes      float64
	IdpTokenEndpoint           string
	ClientID                   string
	ClientSecret               string
	InsecureSkipVerify         bool
	Username                   string
	Password                   string
	VaultPath                  string
	VaultClientIdKey           string
	VaultClientSecretKey       string
	VaultUsernameKey           string
	VaultPasswordKey           string
	VaultIdpTokenEndpointKey   string
	vaultClientIdValue         string
	vaultClientSecretValue     string
	vaultUsernameValue         string
	vaultPasswordValue         string
	vaultIdpTokenEndpointValue string
	vaultEnabled               bool
}

// applyDefaults fills zero values with safe defaults. It is shared by file
// loading and programmatic configuration so both paths behave identically.
func applyDefaults(conf *Config) {
	if conf == nil {
		return
	}
	if conf.Log.Level == "" {
		conf.Log.Level = "info"
	}
	if conf.Log.FilePath == "" {
		conf.Log.FilePath = "./default.log"
	}
	if conf.Log.MaxAge == 0 {
		conf.Log.MaxAge = 10
	}
	if conf.Log.MaxSize == 0 {
		conf.Log.MaxSize = 51200
	}
	if conf.Log.MaxBackups == 0 {
		conf.Log.MaxBackups = 10
	}
	if conf.Database.BoltDB.OpenTimeoutSeconds <= 0 {
		conf.Database.BoltDB.OpenTimeoutSeconds = 5
	}
	if conf.HTTP.Server.ReadHeaderTimeoutSeconds <= 0 {
		conf.HTTP.Server.ReadHeaderTimeoutSeconds = 5
	}
	if conf.HTTP.Server.ReadTimeoutSeconds <= 0 {
		conf.HTTP.Server.ReadTimeoutSeconds = 30
	}
	if conf.HTTP.Server.WriteTimeoutSeconds <= 0 {
		conf.HTTP.Server.WriteTimeoutSeconds = 60
	}
	if conf.HTTP.Server.IdleTimeoutSeconds <= 0 {
		conf.HTTP.Server.IdleTimeoutSeconds = 120
	}
	if conf.HTTP.Server.MaxRequestBodyBytes <= 0 {
		conf.HTTP.Server.MaxRequestBodyBytes = DefaultReadJsonRequestLimit
	}
	if conf.Auth.Server.OAuth.RolesClaim == "" {
		conf.Auth.Server.OAuth.RolesClaim = "roles"
	}
	if conf.Grpc.Server.ReadHeaderTimeoutSeconds <= 0 {
		conf.Grpc.Server.ReadHeaderTimeoutSeconds = 5
	}
	if conf.Grpc.Server.ReadTimeoutSeconds <= 0 {
		conf.Grpc.Server.ReadTimeoutSeconds = 30
	}
	if conf.Grpc.Server.WriteTimeoutSeconds <= 0 {
		conf.Grpc.Server.WriteTimeoutSeconds = 60
	}
	if conf.Grpc.Server.IdleTimeoutSeconds <= 0 {
		conf.Grpc.Server.IdleTimeoutSeconds = 120
	}
	if conf.Metrics.Path == "" {
		conf.Metrics.Path = "/metrics"
	}
}

// validateConfiguration rejects configuration that cannot be used safely.
// File existence checks belong to serving validation, not config parsing.
func validateConfiguration(conf *Config) error {
	if conf == nil {
		return errors.New("config is nil")
	}
	if _, err := logLevelStringToZapType(conf.Log.Level); err != nil {
		return err
	}
	if conf.Metrics.Enabled && conf.Metrics.ListenAddress == "" {
		return errors.New("metrics.enabled requires metrics.listenaddress")
	}
	if err := validateOrigins("http.server.allowedorigins", conf.HTTP.Server.AllowedOrigins); err != nil {
		return err
	}
	if err := validateOrigins("grpc.server.allowedorigins", conf.Grpc.Server.AllowedOrigins); err != nil {
		return err
	}
	if len(conf.Auth.Server.UnauthenticatedPaths) > 0 && len(conf.Grpc.Server.UnAuthenticatedPaths) > 0 {
		return ErrConflictingUnauthenticatedPaths
	}
	return nil
}

// checkPlatformConfiguration applies defaults and validates. Retained as the
// internal compatibility entry point for tests.
func (conf *Config) checkPlatformConfiguration() error {
	if conf == nil {
		return errors.New("config is nil")
	}
	applyDefaults(conf)
	return validateConfiguration(conf)
}

func createDefaultConfiguration() *Config {
	cfg := &Config{}
	cfg.Auth.Server.Basic.Enabled = false
	cfg.Auth.Server.OAuth.Enabled = false
	cfg.HTTP.Server.TLSEnabled = false
	cfg.HTTP.Server.ListeningAddress = "127.0.0.1:10000"
	cfg.Log.Level = "info"
	cfg.Log.FileLoggingEnabled = false
	cfg.Database.BoltDB.Enabled = false
	cfg.Component.ComponentName = "Not Specified"
	applyDefaults(cfg)
	return cfg
}
