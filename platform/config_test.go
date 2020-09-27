package platform

import (
	"log"
	"testing"
)

func TestWriteConfig(t *testing.T) {
	config := Config{}
	config.LogLevel = "info"
	config.Component.ComponentName = "Unit Test"
	config.Component.ComponentConfigFileName = "serviceconfigfile.hcl"
	config.HTTP.Server.ListeningAddress = "0.0.0.0:9111"
	config.HTTP.Server.TLSEnabled = false

	writePlatformConfiguration(config)
}

func TestReadConfig(t *testing.T) {

	config, err := readPlatformConfiguration()
	if err != nil {
		t.Fail()
	}

	log.Println(config)
	log.Println(config.LogLevel)
}
