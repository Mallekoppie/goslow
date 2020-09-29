package platform

import (
	"errors"
)

var (
	Database            PlatformDatabases
	ErrNoEntryFoundInDB = errors.New("No entry found in the database")
)

type PlatformDatabases struct {
	BoltDb BoltDbDatabase
}

func init() {
	Database = PlatformDatabases{}
}
