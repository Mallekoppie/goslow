package platform

import (
	"log"
	"testing"
)

func TestWriteConfig(t *testing.T) {
	config := Config{}
	config.Log.LogLevel = "info"
	config.Component.ComponentName = "Unit Test"
	config.Component.ComponentConfigFileName = "serviceconfigfile.hcl"
	config.HTTP.Server.ListeningAddress = "0.0.0.0:9111"
	config.HTTP.Server.TLSEnabled = false
	config.HTTP.Clients = make([]HTTPClientConfig, 0)
	config.HTTP.Clients = append(config.HTTP.Clients, HTTPClientConfig{ID: "default", RequestTimeout: 10,
		MaxIdleConnections: 10})
	config.HTTP.Clients = append(config.HTTP.Clients, HTTPClientConfig{ID: "custom", RequestTimeout: 10,
		MaxIdleConnections: 10})

	config.Auth.Server.OAuth.AllowedAlgorithms = make([]string, 0)
	config.Auth.Server.OAuth.AllowedAlgorithms = append(config.Auth.Server.OAuth.AllowedAlgorithms, "rs256")
	config.Auth.Server.OAuth.AllowedAlgorithms = append(config.Auth.Server.OAuth.AllowedAlgorithms, "rs384")

	config.Auth.Client.OAuth.OwnTokens = make([]OwnTokenConfig, 0)
	config.Auth.Client.OAuth.OwnTokens = append(config.Auth.Client.OAuth.OwnTokens, OwnTokenConfig{ID: "default", ClientID: "test client ID",
		ClientSecret: "some secret", Username: "test username", Password: "testpassword"})
	config.Auth.Client.OAuth.OwnTokens = append(config.Auth.Client.OAuth.OwnTokens, OwnTokenConfig{ID: "exsternalApi", ClientID: "remoteClientID",
		ClientSecret: "remote secret", Username: "test username", Password: "testpassword"})

	config.Auth.Server.Basic.AllowedUsers = make(map[string]string, 0)
	config.Auth.Server.Basic.AllowedUsers["user1"] = "pass1"
	config.Auth.Server.Basic.AllowedUsers["user2"] = "pass2"

	writePlatformConfiguration(config)
}

func TestReadConfig(t *testing.T) {

	config, err := readPlatformConfiguration()
	if err != nil {
		t.Fail()
	}

	log.Println(config)
	log.Println(config.Log.LogLevel)
}
