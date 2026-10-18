package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func resetPlatformState(t *testing.T) {
	t.Helper()
	mutex.Lock()
	defer mutex.Unlock()
	internalConfig = nil
	dbBoltMu.Lock()
	if dbBolt != nil {
		_ = dbBolt.Close()
		dbBolt = nil
	}
	dbBoltMu.Unlock()
	oAuthTokens = nil
	issuerJwkUrlMap = nil
	viper.Reset()
	Log = nil
}

func seedConfig(t *testing.T) {
	t.Helper()
	conf := createDefaultConfiguration()
	conf.Log.Level = "info"
	conf.Log.FileLoggingEnabled = false
	conf.Database.BoltDB.Enabled = true
	conf.Database.BoltDB.FileName = filepath.Join(t.TempDir(), "bolt.db")
	SetPlatformConfiguration(*conf)
}

func withTestDB(t *testing.T) string {
	t.Helper()
	resetPlatformState(t)
	seedConfig(t)
	path := internalConfig.Database.BoltDB.FileName
	if err := SetupBoltDB(); err != nil {
		t.Fatalf("SetupBoltDB: %v", err)
	}
	return path
}

func TestResetPlatformStateSmoke(t *testing.T) {
	resetPlatformState(t)
	if _, err := os.Stat(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("unexpected file found")
	}
}
