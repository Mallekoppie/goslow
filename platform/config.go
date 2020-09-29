package platform

import (
	"errors"
	"log"
	"sync"

	"github.com/spf13/viper"
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

// Config ... Platform configuration
type config struct {
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
			OAuth struct {
				OwnTokens []ownTokenConfig
			}
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
}

// HTTPClientConfig ... For HTTP client configuration
type httpClientConfig struct {
	ID                 string
	TLSVerify          bool
	MaxIdleConnections int
	RequestTimeout     int
}

// OwnTokenConfig ... Will need to secure the credentials in the future
type ownTokenConfig struct {
	ID              string
	IdpWellKnownURL string
	ClientID        string
	ClientSecret    string
	Username        string
	Password        string
}

func (conf *config) checkPlatformConfiguration() error {
	if len(conf.Log.FilePath) < 1 {
		return ErrInvalidConfigFilePath
	}

	return nil
}
