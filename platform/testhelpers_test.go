package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// TestMain stops the OAuth renewal goroutines before the test binary exits so
// they cannot log into a closing stdout or race with package teardown.
func TestMain(m *testing.M) {
	code := m.Run()
	stopOAuthTokenRenewal()
	os.Exit(code)
}

func resetPlatformState(t *testing.T) {
	t.Helper()

	// Stop background token renewal before clearing the state it depends on.
	stopOAuthTokenRenewal()

	mutex.Lock()
	internalConfig = nil
	mutex.Unlock()

	dbBoltMu.Lock()
	if dbBolt != nil {
		_ = dbBolt.Close()
		dbBolt = nil
	}
	dbBoltMu.Unlock()

	oAuthStateLock.Lock()
	oAuthTokens = nil
	oAuthClientTokenConfiguration = nil
	oAuthStateLock.Unlock()

	issuerJwkUrlMap = nil

	vaultClientList = nil
	vaultEnabled = false
	vaultInitialized = false

	viper.Reset()

	// Never leave a nil logger behind: background code paths log unconditionally.
	ConfigureLoggerWithConfig(createDefaultConfiguration())
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
