package platform

import (
	"fmt"
	"testing"
)

func TestGetOAuth2Token(t *testing.T) {
	token, err := GetToken("default")
	if err != nil {
		t.FailNow()
	}

	fmt.Println(token)
}
