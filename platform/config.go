package platform

import (
	"log"

	"github.com/spf13/viper"
)

func init() {
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")
	viper.SetConfigName("platform")
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

func readPlatformConfiguration() (Config, error) {
	var config Config
	err := viper.ReadInConfig()
	if err != nil {
		log.Println("Unable to read config file: ", err.Error())
		return config, err
	}
	err = viper.UnmarshalKey("platform", &config)
	// err := viper.Unmarshal(&config)
	if err != nil {
		log.Println("Error reading config: ", err.Error())
		return config, err
	}

	return config, nil
}

// Config ... Platform configuration
type Config struct {
	Log struct {
		LogLevel    string
		LogFilePath string
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
		ComponentName           string
		ComponentConfigFileName string
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
