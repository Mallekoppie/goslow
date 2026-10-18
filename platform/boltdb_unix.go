//go:build !windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	bolterrors "go.etcd.io/bbolt/errors"
)

// describeBoltOpenError adds the file details an operator needs when a database
// cannot be opened, while preserving the original error for errors.Is checks.
func describeBoltOpenError(path string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, bolterrors.ErrTimeout) {
		return fmt.Errorf("bolt DB %q is locked by another process (timeout): %w", path, err)
	}

	mode := boltFileMode
	uid, gid := -1, -1
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			uid = int(stat.Uid)
			gid = int(stat.Gid)
		}
	}

	return fmt.Errorf("failed to open bolt DB %q (mode=%04o, uid=%d, gid=%d): %w", path, mode, uid, gid, err)
}
