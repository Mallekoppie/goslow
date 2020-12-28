package platform

import (
	"encoding/json"
	"os"

	"github.com/boltdb/bolt"
	"go.uber.org/zap"
)

var (
	dbBolt *bolt.DB
)

type boltDbDatabase struct {
}

func databaseHackToRestartServiceIKnowThisIsBad(conf *config) {
	os.Chmod(conf.Database.BoltDB.FileName, 0755)
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
		return
	}

	Logger.Debug("Calling open database",
		zap.String("filename", config.Database.BoltDB.FileName))

	// Hack to make the DB file writeable
	databaseHackToRestartServiceIKnowThisIsBad(config)

	dbBolt, err = bolt.Open(config.Database.BoltDB.FileName, os.ModeExclusive, nil)
	if err != nil {
		Logger.Fatal("Error opening database", zap.Error(err))
	}

	Logger.Debug("Boltdb created without error")
}

func (d *boltDbDatabase) Close() error {
	return dbBolt.Close()
}

func (d *boltDbDatabase) SaveObject(bucket string, id string, object interface{}) error {

	Logger.Debug("Saving object to DB",
		zap.String("bucket", bucket),
		zap.String("id", id),
		zap.Any("object", object))

	if dbBolt == nil {
		Logger.Fatal("BoltDB instance is nil")
	}

	err := dbBolt.Update(func(tx *bolt.Tx) error {
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

	err := dbBolt.View(func(tx *bolt.Tx) error {
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

// Returns all entries in the bucket. Values are still json strings
func (d *boltDbDatabase) ReadAllObjects(bucket string) (map[string]string, error) {

	results := make(map[string]string)

	err := dbBolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return ErrNoEntryFoundInDB
		}

		cursor := b.Cursor()

		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			results[string(key)] = string(value)
		}
		return nil
	})
	if err != nil {
		Logger.Error("Error reading from database", zap.Error(err))
		return results, err
	}

	return results, nil
}

func (d *boltDbDatabase) RemoveObject(bucket string, id string) error {

	Logger.Debug("Removing object from bucket",
		zap.String("bucket", bucket),
		zap.String("id", id))

	if dbBolt == nil {
		Logger.Fatal("BoltDB instance is nil")
	}

	err := dbBolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			Logger.Error("Error creating bucket", zap.Error(err))
			return err
		}

		err = b.Delete([]byte(id))
		if err != nil {
			Logger.Error("Error removing key from bucket", zap.String("id", id), zap.String("bucket", bucket))
		}

		return nil
	})

	if err != nil {
		Logger.Error("Error updating DB", zap.Error(err))
		return err
	}

	return nil
}

func (d *boltDbDatabase) RemoveBucket(bucket string) error {

	Logger.Debug("Deleting bucket",
		zap.String("bucket", bucket))

	if dbBolt == nil {
		Logger.Fatal("BoltDB instance is nil")
	}

	err := dbBolt.Update(func(tx *bolt.Tx) error {
		err := tx.DeleteBucket([]byte(bucket))
		if err != nil  && err == bolt.ErrBucketNotFound {
			return nil
		}

		if err != nil {
			Logger.Error("Error removing bucket", zap.Error(err))
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
