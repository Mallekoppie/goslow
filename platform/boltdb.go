package platform

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/boltdb/bolt"
	"go.uber.org/zap"
)

var (
	dbBolt              *bolt.DB
	ErrBoltDBNoDBObject = errors.New("no db object")
)

type boltDbDatabase struct {
}

func databaseHackToRestartServiceIKnowThisIsBad(conf *Config) {
	os.Chmod(conf.Database.BoltDB.FileName, 0755)
	os.Remove(conf.Database.BoltDB.FileName + ".lock")

}

func init() {
	// This is because this init method executes before the Logger.init() method
	// Don't ask me why
	// Its too late
	InitializeLogger()

	Log.Debug("Creating boltdb")
	config, err := GetPlatformConfiguration()
	if err != nil {
		Log.Error("Unable to read platform configuration", zap.Error(err))
		panic(errors.New("unable to get configuration"))
	}
	Log.Debug("Config read completed")
	if config.Database.BoltDB.Enabled == false {
		Log.Info("Database BoltDb not enabled")
		return
	}

	Log.Debug("Calling open database",
		zap.String("filename", config.Database.BoltDB.FileName))

	// Hack to make the DB file writeable
	databaseHackToRestartServiceIKnowThisIsBad(config)

	dbBolt, err = bolt.Open(config.Database.BoltDB.FileName, os.ModeExclusive, nil)
	if err != nil {
		Log.Error("Error opening database", zap.Error(err))
		panic(errors.New("Unable to open db file"))
	}

	Log.Debug("Boltdb created without error")
}

func (d *boltDbDatabase) Close() error {
	return dbBolt.Close()
}

func (d *boltDbDatabase) SaveObject(bucket string, id string, object interface{}) error {

	Log.Debug("Saving object to DB",
		zap.String("bucket", bucket),
		zap.String("id", id),
		zap.Any("object", object))

	if dbBolt == nil {
		Log.Error("BoltDB instance is nil")
		return ErrBoltDBNoDBObject
	}

	err := dbBolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			Log.Error("Error creating bucket", zap.Error(err))
			return err
		}

		data, err := json.Marshal(object)
		if err != nil {
			Log.Error("Error marshalling object", zap.Error(err))
			return err
		}

		err = b.Put([]byte(id), data)
		if err != nil {
			Log.Error("Error adding data", zap.Error(err))
			return err
		}

		return nil
	})

	if err != nil {
		Log.Error("Error updating DB", zap.Error(err))
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
				Log.Error("Error marshalling DB response", zap.Error(err))
				return err
			}

		} else {
			Log.Warn("No entry found in the database", zap.String("id", id))
			return ErrNoEntryFoundInDB
		}

		return nil
	})
	if err != nil {
		Log.Error("Error readign from database", zap.Error(err))
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
		Log.Error("Error reading from database", zap.Error(err))
		return results, err
	}

	return results, nil
}

func (d *boltDbDatabase) RemoveObject(bucket string, id string) error {

	Log.Debug("Removing object from bucket",
		zap.String("bucket", bucket),
		zap.String("id", id))

	if dbBolt == nil {
		Log.Error("BoltDB instance is nil")
		return ErrBoltDBNoDBObject
	}

	err := dbBolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			Log.Error("Error creating bucket", zap.Error(err))
			return err
		}

		err = b.Delete([]byte(id))
		if err != nil {
			Log.Error("Error removing key from bucket", zap.String("id", id), zap.String("bucket", bucket))
		}

		return nil
	})

	if err != nil {
		Log.Error("Error updating DB", zap.Error(err))
		return err
	}

	return nil
}

func (d *boltDbDatabase) RemoveBucket(bucket string) error {

	Log.Debug("Deleting bucket",
		zap.String("bucket", bucket))

	if dbBolt == nil {
		Log.Error("BoltDB instance is nil")
		return ErrBoltDBNoDBObject
	}

	err := dbBolt.Update(func(tx *bolt.Tx) error {
		err := tx.DeleteBucket([]byte(bucket))
		if err != nil && err == bolt.ErrBucketNotFound {
			return nil
		}

		if err != nil {
			Log.Error("Error removing bucket", zap.Error(err))
			return err
		}

		return nil
	})

	if err != nil {
		Log.Error("Error updating DB", zap.Error(err))
		return err
	}

	return nil
}

func (d *boltDbDatabase) RemoveDBFile() error {

	Log.Debug("Removing BoltDB file")

	if dbBolt == nil {
		Log.Error("BoltDB instance is nil")
	}

	err := dbBolt.Close()
	if err != nil {
		Log.Error("Error closing BoltDB", zap.Error(err))
		return err
	}

	err = os.Remove(internalConfig.Database.BoltDB.FileName)
	if err != nil {
		Log.Error("Error removing BoltDB file", zap.Error(err))
		return err
	}

	return nil
}
