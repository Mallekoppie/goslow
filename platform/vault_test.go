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

	log.Println("Retrieved secrets: ", secrets)
}
