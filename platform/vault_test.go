package platform

import (
	"log"
	"testing"
)

// Make sure to update the config file before running the test
func TestLoginWithToken(t *testing.T) {
	secrets, err := Vault.GetSecrets("kv-v2/data/dev/test/creds")
	if err != nil {
		log.Println("Failed to get secrets using token", err.Error())
		t.FailNow()
	}

	username := secrets["username"]
	password := secrets["password"]

	log.Println("Found username: ", username)
	log.Println("Found password: ", password)

	log.Println("Retrieved secrets: ", secrets)
}
