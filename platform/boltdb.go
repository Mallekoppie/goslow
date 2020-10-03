package platform

import (
	"encoding/json"
	"os"

	"github.com/boltdb/bolt"
	"go.uber.org/zap"
)

var (
	db *bolt.DB
)

type boltDbDatabase struct {
}

func databaseHackToRestartServiceIKnowThisIsBad(conf *config) {
	os.Chmod(conf.Database.BoltDB.FileName, 0222)
	os.Remove(conf.Database.BoltDB.FileName + ".lock")

}

func init() {
	// This is because this init method executes before the Logger.init() method
	// Don't ask me why
	// Its too late
	InitializeLogger()

	Logger.Debug("Creating boltdb")
	config, err := getPlatformConfiguration()
	if err != nil {
		Logger.Fatal("Unable to read platform configuration", zap.Error(err))
	}
	Logger.Debug("Config read completed")
	if config.Database.BoltDB.Enabled == false {
		Logger.Info("Database BoltDb not enabled")
	}

	Logger.Debug("Calling open database",
		zap.String("filename", config.Database.BoltDB.FileName))

	// Hack to make the DB file writeable
	databaseHackToRestartServiceIKnowThisIsBad(config)

	db, err = bolt.Open(config.Database.BoltDB.FileName, os.ModeExclusive, nil)
	if err != nil {
		Logger.Fatal("Error opening database", zap.Error(err))
	}

	Logger.Debug("Boltdb created without error")
}

func (d *boltDbDatabase) Close() error {
	return db.Close()
}

func (d *boltDbDatabase) SaveObject(bucket string, id string, object interface{}) error {

	Logger.Debug("Saving object to DB",
		zap.String("bucket", bucket),
		zap.String("id", id),
		zap.Any("object", object))

	if db == nil {
		Logger.Fatal("BoltDB instance is nil")
	}

	err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			Logger.Error("Error creating bucket", zap.Error(err))
			return err
		}

		data, err := json.Marshal(object)
		if err != nil {
			Logger.Error("Error marshalling object", zap.Error(err))
			return err
		}

		err = b.Put([]byte(id), data)
		if err != nil {
			Logger.Error("Error adding data", zap.Error(err))
			return err
		}

		return nil
	})

	if err != nil {
		Logger.Error("Error updating DB", zap.Error(err))
		return err
	}

	return nil
}

func (d *boltDbDatabase) ReadObject(bucket string, id string, object interface{}) error {

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		result := b.Get([]byte(id))
		if len(result) > 0 {
			err := json.Unmarshal(result, &object)
			if err != nil {
				Logger.Error("Error marshalling DB response", zap.Error(err))
				return err
			}

		} else {
			Logger.Warn("No entry found in the database", zap.String("id", id))
			return ErrNoEntryFoundInDB
		}

		return nil
	})
	if err != nil {
		Logger.Error("Error readign from database", zap.Error(err))
		return err
	}

	return nil
}
