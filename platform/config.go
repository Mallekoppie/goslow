package platform

import (
	"errors"
	"log"
	"sync"

	"github.com/spf13/viper"
)

var (
	config                   *Config
	mutex                    sync.Mutex
	ErrInvalidConfigFilePath = errors.New("Invalid config file path for settings platform.log.logfilepath")
)

func init() {
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
}

func writePlatformConfiguration(config Config) error {
	viper.Set("platform", config)

	err := viper.WriteConfig()
	if err != nil {
		log.Println("Error writing config: ", err.Error())
		return err
	}

	return nil
}

func getPlatformConfiguration() (*Config, error) {

	if config == nil {
		mutex.Lock()
		if config == nil {
			err := viper.ReadInConfig()
			if err != nil {
				log.Println("Unable to read config file: ", err.Error())
				return config, err
			}
			err = viper.UnmarshalKey("platform", &config)
			if err != nil {
				log.Println("Error reading config: ", err.Error())
				return config, err
			}
		}

		err := config.checkPlatformConfiguration()
		if err != nil {
			log.Println("Config file incorrect: ", err.Error())
			return config, err
		}

		mutex.Unlock()
	}

	return config, nil
}

// Config ... Platform configuration
type Config struct {
	Log struct {
		Level    string
		FilePath string
	}

	HTTP struct {
		Server struct {
			ListeningAddress string
			TLSCertFileName  string
			TLSKeyFileName   string
			TLSEnabled       bool
		}

		Clients []HTTPClientConfig
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
			OAuth struct {
				OwnTokens []OwnTokenConfig
			}
		}
	}

	Component struct {
		ComponentName string
	}
}

// HTTPClientConfig ... For HTTP client configuration
type HTTPClientConfig struct {
	ID                 string
	TLSVerify          bool
	MaxIdleConnections int
	RequestTimeout     int
}

// OwnTokenConfig ... Will need to secure the credentials in the future
type OwnTokenConfig struct {
	ID              string
	IdpWellKnownURL string
	ClientID        string
	ClientSecret    string
	Username        string
	Password        string
}

func (conf *Config) checkPlatformConfiguration() error {
	if len(conf.Log.FilePath) < 1 {
		return ErrInvalidConfigFilePath
	}

	return nil
}
