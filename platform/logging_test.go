package platform

import (
	"testing"

	"go.uber.org/zap"
)

func TestWriteLogs(t *testing.T) {
	Logger.Info("test",
		zap.String("field", "value"))
	Logger.Error("This is an error")

	Logger.Info("Second line test")
}
