package platform

import (
	"log"
	"testing"
)

// Only uncomment if you need to test the configuration
//func TestWriteConfig(t *testing.T) {
//	config := config{}
//	config.Log.Level = "debug"
//	config.Log.FilePath = "./log.txt"
//	config.Component.ComponentName = "Unit Test"
//	config.HTTP.Server.ListeningAddress = "0.0.0.0:9111"
//	config.HTTP.Server.TLSEnabled = false
//	config.HTTP.Clients = make([]httpClientConfig, 0)
//	config.HTTP.Clients = append(config.HTTP.Clients, httpClientConfig{ID: "default", RequestTimeout: 10,
//		MaxIdleConnections: 10})
//	config.HTTP.Clients = append(config.HTTP.Clients, httpClientConfig{ID: "custom", RequestTimeout: 10,
//		MaxIdleConnections: 10})
//
//	config.Auth.Server.OAuth.AllowedAlgorithms = make([]string, 0)
//	config.Auth.Server.OAuth.AllowedAlgorithms = append(config.Auth.Server.OAuth.AllowedAlgorithms, "rs256")
//	config.Auth.Server.OAuth.AllowedAlgorithms = append(config.Auth.Server.OAuth.AllowedAlgorithms, "rs384")
//
//	config.Auth.Client.OAuth.OwnTokens = make([]clientTokenConfig, 0)
//	config.Auth.Client.OAuth.OwnTokens = append(config.Auth.Client.OAuth.OwnTokens, clientTokenConfig{ID: "default", ClientID: "test client ID",
//		ClientSecret: "some secret", Username: "test username", Password: "testpassword"})
//	config.Auth.Client.OAuth.OwnTokens = append(config.Auth.Client.OAuth.OwnTokens, clientTokenConfig{ID: "exsternalApi", ClientID: "remoteClientID",
//		ClientSecret: "remote secret", Username: "test username", Password: "testpassword"})
//
//	config.Auth.Server.Basic.AllowedUsers = make(map[string]string, 0)
//	config.Auth.Server.Basic.AllowedUsers["user1"] = "pass1"
//	config.Auth.Server.Basic.AllowedUsers["user2"] = "pass2"
//
//	config.Database.BoltDB.Enabled = true
//	config.Database.BoltDB.FileName = "./database.db"
//
//	writePlatformConfiguration(config)
//}

func TestReadConfig(t *testing.T) {

	config, err := GetPlatformConfiguration()
	if err != nil {
		t.Fail()
	}

	log.Println(config)
	log.Println(config.Log.Level)
}
