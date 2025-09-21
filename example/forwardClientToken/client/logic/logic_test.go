package logic

import "testing"

func TestCallServer(t *testing.T) {
	err := CallServer("test-client-token")
	if err != nil {
		t.FailNow()
	}
}
