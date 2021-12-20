package platform

import (
	"errors"
	"log"
	"sync"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	internalConfig           *config
	mutex                    sync.Mutex
	ErrInvalidConfigFilePath = errors.New("Invalid config file path for settings platform.log.logfilepath")
)

func writePlatformConfiguration(conf config) error {
	viper.Set("platform", conf)

	err := viper.WriteConfig()
	if err != nil {
		log.Println("Error writing config: ", err.Error())
		return err
	}

	return nil
}

func getPlatformConfiguration() (*config, error) {

	if internalConfig == nil {
		mutex.Lock()
		if internalConfig == nil {
			viper.SetConfigType("yml")
			viper.AddConfigPath(".")
			viper.SetConfigName("config")

			err := viper.ReadInConfig()
			if err != nil {
				log.Println("Unable to read config file: ", err.Error())
				return internalConfig, err
			}
			err = viper.UnmarshalKey("platform", &internalConfig)
			if err != nil {
				log.Println("Error reading config: ", err.Error())
				return internalConfig, err
			}
		}

		err := internalConfig.checkPlatformConfiguration()
		if err != nil {
			log.Println("Config file incorrect: ", err.Error())
			return internalConfig, err
		}

		mutex.Unlock()
	}

	return internalConfig, nil
}

func GetComponentConfiguration(key string, object interface{}) error {
	err := viper.UnmarshalKey(key, &object)
	if err != nil {
		Logger.Error("Unable to read component configuration", zap.String("configkey", key), zap.Error(err))
		return err
	}

	return nil
}

// Config ... Platform configuration
type config struct {
	Log struct {
		Level    string
		FilePath string
		//MegaBytes
		MaxSize    int
		MaxBackups int
		// Days
		MaxAge int
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

	Auth struct {
		Server struct {
			OAuth struct {
				Enabled           bool
				IdpWellKnownURL   string
				ClientID          string
				AllowedAlgorithms []string
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

// HTTPClientConfig ... For HTTP client configuration
type httpClientConfig struct {
	ID                 string
	TLSVerify          bool
	MaxIdleConnections int
	RequestTimeout     int
}

// OwnTokenConfig ... Will need to secure the credentials in the future
type clientTokenConfig struct {
	ID                        string
	IdpWellKnownURL           string
	RenewCheckIntervalSeconds float64
	RenewCheckTimeMinutes     float64
	IdpTokenEndpoint          string
	ClientID                  string
	ClientSecret              string
	Username                  string
	Password                  string
	VaultPath                 string
	VaultClientIdKey          string
	VaultClientSecretKey      string
	VaultUsernameKey          string
	VaultPasswordKey          string
	VaultIdpTokenEndpointKey  string

	// Internal for the retrieved Vault values
	vaultClientIdValue         string
	vaultClientSecretValue     string
	vaultUsernameValue         string
	vaultPasswordValue         string
	vaultIdpTokenEndpointValue string
	vaultEnabled               bool
}

func (conf *config) checkPlatformConfiguration() error {
	if len(conf.Log.FilePath) < 1 {
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

	return nil
}
