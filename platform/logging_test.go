package platform

import (
	"testing"

	"go.uber.org/zap"
)

func TestWriteLogs(t *testing.T) {
	Log.Info("test",
		zap.String("field", "value"))
	Log.Error("This is an error")

	Log.Info("Second line test")
}

//func TestCheckForFileRotation(t *testing.T){
//	for i := 0; i < 2000000; i++ {
//		Log.Info("We are testing the log rotation")
//	}
//}
