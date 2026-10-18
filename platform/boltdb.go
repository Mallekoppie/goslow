package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	bbolt "go.etcd.io/bbolt"
	"go.uber.org/zap"
)

var (
	dbBolt              *bbolt.DB
	dbBoltMu            sync.RWMutex
	ErrBoltDBNoDBObject = errors.New("no db object")
	boltFileMode        = os.FileMode(0o600)
	defaultOpenTimeout  = 5 * time.Second
)

type boltDbDatabase struct{}

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
	dbBoltMu.RLock()
	open := dbBolt != nil
	dbBoltMu.RUnlock()
	if open {
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

	timeout := defaultOpenTimeout
	if conf.Database.BoltDB.OpenTimeoutSeconds > 0 {
		timeout = time.Duration(conf.Database.BoltDB.OpenTimeoutSeconds) * time.Second
	}

	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt != nil {
		return nil
	}
	opened, err := OpenBoltDB(fileName, timeout)
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
	return DeleteDatabase(fileName)
}

// DeleteDatabase removes the bolt database at the supplied path. The open handle is
// only closed when it points at the same file, so callers are explicit about what
// they delete.
func DeleteDatabase(path string) error {
	if path == "" {
		return errors.New("bolt db path is empty")
	}

	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()

	if dbBolt != nil && dbBolt.Path() == path {
		if err := dbBolt.Close(); err != nil {
			return err
		}
		dbBolt = nil
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// View runs a read-only transaction. The callback must not call other database
// methods, because the handle lock is held for the transaction duration.
func (d *boltDbDatabase) View(fn func(tx *bbolt.Tx) error) error {
	if fn == nil {
		return errors.New("nil view function")
	}
	if err := ensureBoltDB(); err != nil {
		return err
	}

	dbBoltMu.RLock()
	defer dbBoltMu.RUnlock()
	if dbBolt == nil {
		return ErrBoltDBNoDBObject
	}
	return dbBolt.View(fn)
}

// Update runs a read-write transaction. The callback must not call other database
// methods, because the handle lock is held for the transaction duration.
func (d *boltDbDatabase) Update(fn func(tx *bbolt.Tx) error) error {
	if fn == nil {
		return errors.New("nil update function")
	}
	if err := ensureBoltDB(); err != nil {
		return err
	}

	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt == nil {
		return ErrBoltDBNoDBObject
	}
	return dbBolt.Update(fn)
}

// Snapshot writes a consistent copy of the database to the supplied writer. The
// read transaction stays open for the whole copy, so the writer should be fast
// (typically a file).
func (d *boltDbDatabase) Snapshot(w io.Writer) error {
	if w == nil {
		return errors.New("nil snapshot writer")
	}
	return d.View(func(tx *bbolt.Tx) error {
		_, err := tx.WriteTo(w)
		return err
	})
}

// Integrity verifies the database structure and returns the first inconsistency.
func (d *boltDbDatabase) Integrity() error {
	return d.View(func(tx *bbolt.Tx) error {
		for err := range tx.Check() {
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Stats returns a copy of the bolt database statistics.
func (d *boltDbDatabase) Stats() (bbolt.Stats, error) {
	if err := ensureBoltDB(); err != nil {
		return bbolt.Stats{}, err
	}

	dbBoltMu.RLock()
	defer dbBoltMu.RUnlock()
	if dbBolt == nil {
		return bbolt.Stats{}, ErrBoltDBNoDBObject
	}
	return dbBolt.Stats(), nil
}

// SaveObjects stores several objects in a single transaction.
func (d *boltDbDatabase) SaveObjects(bucket string, objects map[string]interface{}) error {
	if len(objects) == 0 {
		return nil
	}
	if err := ensureBoltDB(); err != nil {
		return err
	}

	dbBoltMu.Lock()
	defer dbBoltMu.Unlock()
	if dbBolt == nil {
		return ErrBoltDBNoDBObject
	}
	return dbBolt.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		for id, object := range objects {
			data, err := json.Marshal(object)
			if err != nil {
				return err
			}
			if err := b.Put([]byte(id), data); err != nil {
				return err
			}
		}
		return nil
	})
}

// Keys returns the bucket keys in lexicographic order.
func (d *boltDbDatabase) Keys(bucket string) ([]string, error) {
	keys := make([]string, 0)
	err := d.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return ErrNoEntryFoundInDB
		}
		return b.ForEach(func(key, _ []byte) error {
			keys = append(keys, string(key))
			return nil
		})
	})
	return keys, err
}

// Scan calls fn for every entry in the bucket in key order.
func (d *boltDbDatabase) Scan(bucket string, fn func(key, value []byte) error) error {
	if fn == nil {
		return errors.New("nil scan function")
	}
	return d.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return ErrNoEntryFoundInDB
		}
		return b.ForEach(fn)
	})
}

// ScanPrefix calls fn for every entry whose key starts with prefix, in key order.
func (d *boltDbDatabase) ScanPrefix(bucket, prefix string, fn func(key, value []byte) error) error {
	if fn == nil {
		return errors.New("nil scan function")
	}
	return d.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return ErrNoEntryFoundInDB
		}
		prefixBytes := []byte(prefix)
		cursor := b.Cursor()
		for key, value := cursor.Seek(prefixBytes); key != nil && bytes.HasPrefix(key, prefixBytes); key, value = cursor.Next() {
			if err := fn(key, value); err != nil {
				return err
			}
		}
		return nil
	})
}

// Count returns the number of entries in the bucket.
func (d *boltDbDatabase) Count(bucket string) (int, error) {
	count := 0
	err := d.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return ErrNoEntryFoundInDB
		}
		return b.ForEach(func(_, _ []byte) error {
			count++
			return nil
		})
	})
	return count, err
}
