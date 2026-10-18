package platform

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	bbolt "go.etcd.io/bbolt"
)

func seedPeople(t *testing.T) {
	t.Helper()

	people := map[string]interface{}{
		"zed":   map[string]string{"name": "Zed"},
		"alice": map[string]string{"name": "Alice"},
		"bob":   map[string]string{"name": "Bob"},
	}
	if err := Database.BoltDb.SaveObjects("people", people); err != nil {
		t.Fatalf("SaveObjects failed: %v", err)
	}
}

func TestSaveObjectsKeysAndCount(t *testing.T) {
	withTestDB(t)
	seedPeople(t)

	keys, err := Database.BoltDb.Keys("people")
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	want := []string{"alice", "bob", "zed"}
	if len(keys) != len(want) {
		t.Fatalf("expected %d keys, got %v", len(want), keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("expected lexicographic keys %v, got %v", want, keys)
		}
	}

	count, err := Database.BoltDb.Count("people")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != len(want) {
		t.Fatalf("expected count %d, got %d", len(want), count)
	}

	if _, err := Database.BoltDb.Keys("missing"); !errors.Is(err, ErrNoEntryFoundInDB) {
		t.Fatalf("expected ErrNoEntryFoundInDB for a missing bucket, got %v", err)
	}
	if _, err := Database.BoltDb.Count("missing"); !errors.Is(err, ErrNoEntryFoundInDB) {
		t.Fatalf("expected ErrNoEntryFoundInDB for a missing bucket, got %v", err)
	}
}

func TestScanAndScanPrefix(t *testing.T) {
	withTestDB(t)

	if err := Database.BoltDb.SaveObjects("items", map[string]interface{}{
		"app:1": "one",
		"app:2": "two",
		"web:1": "three",
	}); err != nil {
		t.Fatalf("SaveObjects failed: %v", err)
	}

	var all []string
	if err := Database.BoltDb.Scan("items", func(key, value []byte) error {
		all = append(all, string(key))
		return nil
	}); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 scanned keys, got %v", all)
	}

	var prefixed []string
	if err := Database.BoltDb.ScanPrefix("items", "app:", func(key, value []byte) error {
		prefixed = append(prefixed, string(key))
		return nil
	}); err != nil {
		t.Fatalf("ScanPrefix failed: %v", err)
	}
	if len(prefixed) != 2 || prefixed[0] != "app:1" || prefixed[1] != "app:2" {
		t.Fatalf("unexpected prefix scan result: %v", prefixed)
	}

	sentinel := errors.New("stop")
	if err := Database.BoltDb.Scan("items", func(key, value []byte) error {
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the scan callback error to propagate, got %v", err)
	}
}

func TestSnapshotProducesReopenableCopy(t *testing.T) {
	withTestDB(t)
	seedPeople(t)

	var buffer bytes.Buffer
	if err := Database.BoltDb.Snapshot(&buffer); err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	copyPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(copyPath, buffer.Bytes(), 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	copied, err := bbolt.Open(copyPath, 0o600, nil)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer copied.Close()

	if err := copied.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("people"))
		if b == nil {
			t.Fatal("people bucket missing from snapshot")
		}
		if b.Get([]byte("alice")) == nil {
			t.Fatal("alice missing from snapshot")
		}
		return nil
	}); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	if err := Database.BoltDb.Snapshot(nil); err == nil {
		t.Fatal("expected an error for a nil snapshot writer")
	}
}

func TestIntegrityStatsAndTransactions(t *testing.T) {
	withTestDB(t)
	seedPeople(t)

	if err := Database.BoltDb.Integrity(); err != nil {
		t.Fatalf("Integrity failed: %v", err)
	}

	stats, err := Database.BoltDb.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.TxN == 0 {
		t.Log("no transaction stats recorded yet; statistics object still returned")
	}

	// View exposes a read transaction to callers.
	if err := Database.BoltDb.View(func(tx *bbolt.Tx) error {
		if tx.Bucket([]byte("people")) == nil {
			t.Fatal("expected people bucket in view transaction")
		}
		return nil
	}); err != nil {
		t.Fatalf("View failed: %v", err)
	}

	// Update exposes a write transaction to callers.
	if err := Database.BoltDb.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("audit"))
		if err != nil {
			return err
		}
		return b.Put([]byte("entry"), []byte(`"value"`))
	}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	var audit string
	if err := Database.BoltDb.ReadObject("audit", "entry", &audit); err != nil {
		t.Fatalf("expected the update transaction to persist, got %v", err)
	}
	if audit != "value" {
		t.Fatalf("expected the updated value, got %q", audit)
	}

	if err := Database.BoltDb.View(nil); err == nil {
		t.Fatal("expected an error for a nil view function")
	}
	if err := Database.BoltDb.Update(nil); err == nil {
		t.Fatal("expected an error for a nil update function")
	}
}

func TestDeleteDatabaseOnlyClosesMatchingPath(t *testing.T) {
	path := withTestDB(t)
	seedPeople(t)

	unrelated := filepath.Join(t.TempDir(), "unrelated.db")
	if err := DeleteDatabase(unrelated); err != nil {
		t.Fatalf("deleting a missing unrelated file should be a no-op: %v", err)
	}

	// The open handle must still work because the paths did not match.
	if _, err := Database.BoltDb.Count("people"); err != nil {
		t.Fatalf("expected the open database to keep working: %v", err)
	}

	if err := DeleteDatabase(path); err != nil {
		t.Fatalf("DeleteDatabase failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %q to be removed, stat err=%v", path, err)
	}
}
