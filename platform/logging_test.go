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

//func TestCheckForFileRotation(t *testing.T){
//	for i := 0; i < 2000000; i++ {
//		Logger.Info("We are testing the log rotation")
//	}
//}
