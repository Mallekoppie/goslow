package platform

import (
	"errors"
)

var (
	// Database exposes the configured database handles. The BoltDb value is a
	// zero-value handle; it opens lazily on first use.
	Database              = PlatformDatabases{BoltDb: boltDbDatabase{}}
	ErrNoEntryFoundInDB   = errors.New("No entry found in the database")
	ErrBoltDbIsNotEnabled = errors.New("Bolt DB is not enabled")
)

type PlatformDatabases struct {
	BoltDb boltDbDatabase
}
