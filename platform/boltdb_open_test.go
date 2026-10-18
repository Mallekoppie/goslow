package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bbolt "go.etcd.io/bbolt"
)

func TestOpenBoltDBCreatesParentDirectoryWithRestrictedMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "open.db")

	db, err := OpenBoltDB(path, time.Second)
	if err != nil {
		t.Fatalf("OpenBoltDB failed: %v", err)
	}
	defer db.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if mode := info.Mode().Perm(); mode != boltFileMode {
		t.Fatalf("expected file mode %04o, got %04o", boltFileMode, mode)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat parent directory: %v", err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0o750 {
		t.Fatalf("expected directory mode 0750, got %04o", mode)
	}
}

func TestOpenBoltDBRejectsEmptyPath(t *testing.T) {
	if _, err := OpenBoltDB("", time.Second); err == nil {
		t.Fatal("expected an error for an empty path")
	}
}

func TestOpenBoltDBReopenSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")

	first, err := OpenBoltDB(path, time.Second)
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}

	second, err := OpenBoltDB(path, time.Second)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}

func TestOpenBoltDBLockTimeoutIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.db")

	held, err := OpenBoltDB(path, time.Second)
	if err != nil {
		t.Fatalf("initial open failed: %v", err)
	}
	defer held.Close()

	start := time.Now()
	_, err = OpenBoltDB(path, 200*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the second open to fail while the file is locked")
	}
	if !errors.Is(err, bbolt.ErrTimeout) {
		t.Fatalf("expected bbolt.ErrTimeout in the chain, got %v", err)
	}
	if !strings.Contains(err.Error(), "locked by another process") {
		t.Fatalf("expected a lock hint in the error, got %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("expected the path in the error, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("lock wait took too long: %s", elapsed)
	}
}

func TestOpenBoltDBPermissionErrorKeepsCause(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks are not meaningful when running as root")
	}

	dir := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := OpenBoltDB(filepath.Join(dir, "denied.db"), time.Second)
	if err == nil {
		t.Fatal("expected a permission error")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected os.ErrPermission in the chain, got %v", err)
	}
	for _, want := range []string{dir, "mode=", "uid=", "gid="} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in error %q", want, err.Error())
		}
	}
}

func TestSetupBoltDBDisabledReturnsSentinel(t *testing.T) {
	resetPlatformState(t)

	conf := createDefaultConfiguration()
	conf.Database.BoltDB.Enabled = false
	SetPlatformConfiguration(*conf)

	if err := SetupBoltDB(); !errors.Is(err, ErrBoltDbIsNotEnabled) {
		t.Fatalf("expected ErrBoltDbIsNotEnabled, got %v", err)
	}
}
