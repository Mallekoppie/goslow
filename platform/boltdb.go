package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bbolt "go.etcd.io/bbolt"
	"go.uber.org/zap"
)

var (
	dbBolt              *bbolt.DB
	dbBoltMu            sync.Mutex
	ErrBoltDBNoDBObject = errors.New("no db object")
	boltFileMode        = os.FileMode(0o600)
	defaultOpenTimeout  = 5 * time.Second
)

type boltDbDatabase struct{}

func describeBoltOpenError(path string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, bbolt.ErrTimeout) {
		return fmt.Errorf("bolt DB %q is locked by another process: %w", path, err)
	}
	return fmt.Errorf("failed to open bolt DB %q (mode=%#o): %w", path, boltFileMode, err)
}

func OpenBoltDB(path string, timeout time.Duration) (*bbolt.DB, error) {
	if path == "" {
		return nil, errors.New("bolt db path is empty")
	}
	if timeout <= 0 {
		timeout = defaultOpenTimeout
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
	}
	db, err := bbolt.Open(path, boltFileMode, &bbolt.Options{Timeout: timeout})
	if err != nil {
		return nil, describeBoltOpenError(path, err)
	}
	return db, nil
}

func ensureBoltDB() error {
	if dbBolt != nil {
		return nil
	}
	conf, err := GetPlatformConfiguration()
	if err != nil {
		return err
	}
	if conf == nil || !conf.Database.BoltDB.Enabled {
		if Log != nil {
			Log.Info("Database BoltDb not enabled")
		}
		return ErrBoltDbIsNotEnabled
	}
	return SetupBoltDB()
}

func SetupBoltDB() error {
	conf, err := GetPlatformConfiguration()
	if err != nil {
		return err
	}
	if conf == nil || !conf.Database.BoltDB.Enabled {
		if Log != nil {
			Log.Info("Database BoltDb not enabled")
		}
		return ErrBoltDbIsNotEnabled
	}
	fileName := conf.Database.BoltDB.FileName
	if fileName == "" {
		fileName = "./bolt.db"
	}
	if Log != nil {
		Log.Debug("Setting up BoltDB", zap.String("filename", fileName))
	}

	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt != nil {
		return nil
	}
	opened, err := OpenBoltDB(fileName, defaultOpenTimeout)
	if err != nil {
		if Log != nil {
			Log.Error("Error opening database", zap.Error(err))
		}
		return err
	}
	dbBolt = opened
	if Log != nil {
		Log.Debug("BoltDB setup completed successfully")
	}
	return nil
}

func (d *boltDbDatabase) Close() error {
	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt == nil {
		return nil
	}
	err := dbBolt.Close()
	dbBolt = nil
	return err
}

func (d *boltDbDatabase) SaveObject(bucket string, id string, object interface{}) error {
	if Log != nil {
		Log.Debug("Saving object to DB", zap.String("bucket", bucket), zap.String("id", id), zap.Any("object", object))
	}
	if err := ensureBoltDB(); err != nil {
		return err
	}

	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt == nil {
		if Log != nil {
			Log.Error("BoltDB instance is nil")
		}
		return ErrBoltDBNoDBObject
	}
	return dbBolt.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			if Log != nil {
				Log.Error("Error creating bucket", zap.Error(err))
			}
			return err
		}
		data, err := json.Marshal(object)
		if err != nil {
			if Log != nil {
				Log.Error("Error marshalling object", zap.Error(err))
			}
			return err
		}
		if err := b.Put([]byte(id), data); err != nil {
			if Log != nil {
				Log.Error("Error adding data", zap.Error(err))
			}
			return err
		}
		return nil
	})
}

func (d *boltDbDatabase) ReadObject(bucket string, id string, object interface{}) error {
	if object == nil {
		return errors.New("nil object target")
	}
	if err := ensureBoltDB(); err != nil {
		return err
	}
	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt == nil {
		if Log != nil {
			Log.Error("BoltDB instance is nil")
		}
		return ErrBoltDBNoDBObject
	}
	return dbBolt.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			if Log != nil {
				Log.Warn("No entry found in the database", zap.String("id", id))
			}
			return ErrNoEntryFoundInDB
		}
		result := b.Get([]byte(id))
		if result == nil {
			if Log != nil {
				Log.Warn("No entry found in the database", zap.String("id", id))
			}
			return ErrNoEntryFoundInDB
		}
		if err := json.Unmarshal(result, object); err != nil {
			if Log != nil {
				Log.Error("Error marshalling DB response", zap.Error(err))
			}
			return err
		}
		return nil
	})
}

func (d *boltDbDatabase) ReadAllObjects(bucket string) (map[string]string, error) {
	results := make(map[string]string)
	if err := ensureBoltDB(); err != nil {
		return results, err
	}
	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt == nil {
		return results, ErrBoltDBNoDBObject
	}
	if err := dbBolt.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return ErrNoEntryFoundInDB
		}
		cursor := b.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			results[string(key)] = string(value)
		}
		return nil
	}); err != nil {
		if Log != nil {
			Log.Error("Error reading from database", zap.Error(err))
		}
		return results, err
	}
	return results, nil
}

func (d *boltDbDatabase) RemoveObject(bucket string, id string) error {
	if Log != nil {
		Log.Debug("Removing object from bucket", zap.String("bucket", bucket), zap.String("id", id))
	}
	if err := ensureBoltDB(); err != nil {
		return err
	}
	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt == nil {
		if Log != nil {
			Log.Error("BoltDB instance is nil")
		}
		return ErrBoltDBNoDBObject
	}
	return dbBolt.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			if Log != nil {
				Log.Error("Error creating bucket", zap.Error(err))
			}
			return err
		}
		if err = b.Delete([]byte(id)); err != nil {
			if Log != nil {
				Log.Error("Error removing key from bucket", zap.String("id", id), zap.String("bucket", bucket))
			}
		}
		return nil
	})
}

func (d *boltDbDatabase) RemoveBucket(bucket string) error {
	if Log != nil {
		Log.Debug("Deleting bucket", zap.String("bucket", bucket))
	}
	if err := ensureBoltDB(); err != nil {
		return err
	}
	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt == nil {
		if Log != nil {
			Log.Error("BoltDB instance is nil")
		}
		return ErrBoltDBNoDBObject
	}
	return dbBolt.Update(func(tx *bbolt.Tx) error {
		err := tx.DeleteBucket([]byte(bucket))
		if err != nil && err == bbolt.ErrBucketNotFound {
			return nil
		}
		if err != nil {
			if Log != nil {
				Log.Error("Error removing bucket", zap.Error(err))
			}
			return err
		}
		return nil
	})
}

func (d *boltDbDatabase) RemoveDBFile() error {
	if Log != nil {
		Log.Debug("Removing BoltDB file")
	}
	conf, err := GetPlatformConfiguration()
	if err != nil || conf == nil {
		return err
	}
	fileName := conf.Database.BoltDB.FileName
	if fileName == "" {
		fileName = "./bolt.db"
	}
	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt != nil {
		if err := dbBolt.Close(); err != nil {
			if Log != nil {
				Log.Error("Error closing BoltDB", zap.Error(err))
			}
			return err
		}
		dbBolt = nil
	}
	if err := os.Remove(fileName); err != nil && !os.IsNotExist(err) {
		if Log != nil {
			Log.Error("Error removing BoltDB file", zap.Error(err))
		}
		return err
	}
	return nil
}
