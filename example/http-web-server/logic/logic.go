package logic

import "github.com/Mallekoppie/goslow/platform"

func PlatformDefaultsTest() {
	// This is created to test that we can run a unit test with no config file in the directory
	platform.Logger.Info("Log")
}
