package platform

import (
	"os"
	"testing"
)

func TestStoreAndReadObjectInBoltDB(t *testing.T) {
	withTestDB(t)

	bucketName := "test"
	id := "123123"
	initialObject := testObject{
		Id:      id,
		Name:    "Test name",
		Surname: "Test surname",
	}

	Log.Debug("Calling SaveObject from test")
	if err := Database.BoltDb.SaveObject(bucketName, id, initialObject); err != nil {
		t.Fatalf("SaveObject failed: %v", err)
	}

	resultObject := testObject{}
	if err := Database.BoltDb.ReadObject(bucketName, id, &resultObject); err != nil {
		t.Fatalf("ReadObject failed: %v", err)
	}

	if initialObject != resultObject {
		t.Fatalf("expected %+v, got %+v", initialObject, resultObject)
	}
}

type testObject struct {
	Id      string
	Name    string
	Surname string
}

func TestDBCleanup(t *testing.T) {
	path := withTestDB(t)

	testItem := testObject{
		Id:      "testItem",
		Name:    "Test Name",
		Surname: "Test Surname",
	}

	if err := Database.BoltDb.SaveObject("test", "testItem", testItem); err != nil {
		t.Fatalf("SaveObject failed: %v", err)
	}

	resultObject := testObject{}
	if err := Database.BoltDb.ReadObject("test", "testItem", &resultObject); err != nil {
		t.Fatalf("ReadObject failed: %v", err)
	}
	if resultObject.Id != "testItem" {
		t.Fatalf("expected testItem, got %q", resultObject.Id)
	}

	if err := Database.BoltDb.RemoveDBFile(); err != nil {
		t.Fatalf("RemoveDBFile failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected DB file %q to be removed, stat err=%v", path, err)
	}
}
