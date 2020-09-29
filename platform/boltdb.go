package platform

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/boltdb/bolt"
	"go.uber.org/zap"
)

var (
	boltDb                *bolt.DB
	ErrBoltDbIsNotEnabled = errors.New("Bolt DB is not enabled")
)

type BoltDbDatabase struct {
}

func init() {
	// This is because this init method executes before the Logger.init() method
	// Don't ask me why
	// Its too late
	InitializeLogger()
}

func getDB() (*bolt.DB, error) {
	if boltDb == nil {
		config, err := getPlatformConfiguration()
		if err != nil {
			Logger.Fatal("Unable to read platform configuration", zap.Error(err))
		}

		if config.Database.BoltDB.Enabled == false {
			Logger.Info("Database BoltDb not enabled")
			return nil, ErrBoltDbIsNotEnabled
		}

		db, err := bolt.Open(config.Database.BoltDB.FileName, os.ModeExclusive, nil)
		if err != nil {
			Logger.Fatal("Error opening database", zap.Error(err))
			return nil, err
		}

		boltDb = db

		return db, nil
	} else {
		return boltDb, nil
	}

}

func (d *BoltDbDatabase) SaveObject(bucket string, id string, object interface{}) error {
	db, err := getDB()
	if err != nil {
		Logger.Fatal("Error while getting DB", zap.Error(err))
	}

	err = db.Update(func(tx *bolt.Tx) error {
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

func (d *BoltDbDatabase) ReadObject(bucket string, id string, object interface{}) error {
	db, err := getDB()
	if err != nil {
		Logger.Fatal("Error while getting DB", zap.Error(err))
	}

	err = db.View(func(tx *bolt.Tx) error {
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
