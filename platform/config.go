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
	LogLevel string

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
		OAuthEnabled    bool
		IdpWellKnownURL string

		OwnTokens struct {
			ClientID     string
			ClientSecret string
		}
	}

	Component struct {
		ComponentName           string
		ComponentConfigFileName string
	}
}

// HTTPClientConfig ... For HTTP client configuration
type HTTPClientConfig struct {
	ID        string
	TLSVerify bool
}
