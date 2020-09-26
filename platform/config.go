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

func writePlatformConfiguration(config PlatformConfig) error {
	viper.Set("platform", config)

	err := viper.WriteConfig()
	if err != nil {
		log.Println("Error writing config: ", err.Error())
		return err
	}

	return nil
}

func readPlatformConfiguration() (PlatformConfig, error) {
	var config PlatformConfig
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

type PlatformConfig struct {
	LogLevel string

	Http struct {
		ListeningAddress string
		TlsCertFileName  string
		TlsKeyFileName   string
		TlsEnabled       bool
	}

	Auth struct {
		OAuthEnabled    bool
		IdpWellKnownUrl string

		OwnTokens struct {
			ClientId     string
			ClientSecret string
		}
	}

	Component struct {
		ComponentName           string
		ComponentConfigFileName string
	}
}
