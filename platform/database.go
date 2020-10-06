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
	Database := PlatformDatabases{}
	Database.BoltDb = boltDbDatabase{}
	// err := Database.BoltDb.createBoltDatabase()
	// Database.BoltDb.createBoltDatabase()
	// if err == ErrBoltDbIsNotEnabled {
	// 	// we do nothing
	// } else {
	// 	Logger.Fatal("Error creating BoltDB database", zap.)
	// }
}

type PlatformDatabases struct {
	BoltDb boltDbDatabase
}
