package platform

import (
	"testing"

	"go.uber.org/zap"
)

func TestWriteLogs(t *testing.T) {
	log, err := getLogger("my test", "debug", "")
	if err != nil {
		t.Fail()
	}

	log.Info("test",
		zap.String("field", "value"))
	log.Error("This is an error")

	log, err = getLogger("my test 2", "warn", "")

	log.Info("Second line test")
}
