package logic

import "testing"

func TestCallServer(t *testing.T) {
	err := CallServer()
	if err != nil {
		t.FailNow()
	}
}
