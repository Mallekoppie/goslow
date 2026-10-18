package platform

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	internalConfig           *Config
	mutex                    sync.Mutex
	ErrInvalidConfigFilePath = errors.New("Invalid config file path for settings platform.log.logfilepath")
)

func writePlatformConfiguration(conf Config) error {
	viper.Set("platform", conf)
	return viper.WriteConfig()
}

func SetPlatformConfiguration(conf Config) {
	mutex.Lock()
	internalConfig = &conf
	mutex.Unlock()
	ConfigureLogger()
}

func GetPlatformConfiguration() (*Config, error) {
	mutex.Lock()
	defer mutex.Unlock()

	if internalConfig != nil {
		return internalConfig, nil
	}

	cfg := &Config{}
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./platform")
	viper.SetConfigName("config")

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			fmt.Println("Config file `config.yml` not found. Using default configurations")
			cfg = createDefaultConfiguration()
		} else {
			log.Println("Unable to read config file: ", err.Error())
			return nil, err
		}
	} else if err := viper.UnmarshalKey("platform", cfg); err != nil {
		log.Println("Error reading config: ", err.Error())
		return nil, err
	}

	if err := cfg.checkPlatformConfiguration(); err != nil {
		log.Println("Config file incorrect: ", err.Error())
		return nil, err
	}

	internalConfig = cfg
	return internalConfig, nil
}

func GetComponentConfiguration(key string, object interface{}) error {
	err := viper.UnmarshalKey(key, &object)
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
		}
		Clients []httpClientConfig
	}

	Grpc struct {
		Server struct {
			ListeningAddress     string
			TLSCertFileName      string
			TLSKeyFileName       string
			TLSEnabled           bool
			UnAuthenticatedPaths []string
		}
	}

	Auth struct {
		Server struct {
			OAuth struct {
				Enabled           bool
				IdpWellKnownURL   string
				ClientID          string
				AllowedAlgorithms []string
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
			Enabled  bool
			FileName string
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

type clientTokenConfig struct {
	ID                         string
	IdpWellKnownURL            string
	RenewCheckIntervalSeconds  float64
	RenewCheckTimeMinutes      float64
	IdpTokenEndpoint           string
	ClientID                   string
	ClientSecret               string
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

func (conf *Config) checkPlatformConfiguration() error {
	if conf == nil {
		return errors.New("config is nil")
	}
	if conf.Log.Level == "" {
		conf.Log.Level = "info"
	}
	if conf.Log.FilePath == "" {
		log.Println("Configuration Log.FiePath is empty. Defaulting to ./default.log")
		conf.Log.FilePath = "./default.log"
	}
	if conf.Log.MaxAge == 0 {
		log.Println("Configuration Log.MaxAge is empty. Defaulting to 10")
		conf.Log.MaxAge = 10
	}
	if conf.Log.MaxSize == 0 {
		log.Println("Configuration Log.MaxSize is empty. Defaulting to 51200")
		conf.Log.MaxSize = 51200
	}
	if conf.Log.MaxBackups == 0 {
		conf.Log.MaxBackups = 10
	}
	return nil
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
	cfg.checkPlatformConfiguration()
	return cfg
}
