package platform

import (
	"errors"
)

var (
	Database              PlatformDatabases
	ErrNoEntryFoundInDB   = errors.New("No entry found in the database")
	ErrBoltDbIsNotEnabled = errors.New("Bolt DB is not enabled")
)

func init() {
	Database = PlatformDatabases{BoltDb: boltDbDatabase{}}
}

type PlatformDatabases struct {
	BoltDb boltDbDatabase
}
