package platform

import (
	"log"
	"os"
	"testing"
)

func TestStoreAndReadObjectInBoltDB(t *testing.T) {
	os.Remove("./database.db")
	os.Remove("./database.db.lock")

	// db, err := InitializePlatformDatabases()
	// if err != nil {
	// 	t.Fail()
	// }
	// defer db.BoltDb.Close()

	bucketName := "test"
	id := "123123"
	initailobject := testObject{
		Id:      id,
		Name:    "Test name",
		Surname: "Test surname",
	}

	Log.Debug("Calling SaveObject from test")
	err := Database.BoltDb.SaveObject("test", id, initailobject)
	if err != nil {
		log.Println("Error saving object: ", err.Error())
		t.Fail()
	}

	resultObject := testObject{}
	err = Database.BoltDb.ReadObject(bucketName, id, &resultObject)
	if err != nil {
		log.Println("Error reading object from DB: ", err.Error())
		t.Fail()
	}

	if initailobject.Id != resultObject.Id {
		log.Println("Objects aren't the same. Test failing")
		t.Fail()
	}

}

type testObject struct {
	Id      string
	Name    string
	Surname string
}

func TestDBCleanup(t *testing.T) {

	var testItem = testObject{
		Id:      "testItem",
		Name:    "Test Name",
		Surname: "Test Surname",
	}

	err := Database.BoltDb.SaveObject("test", "testItem", testItem)
	if err != nil {
		log.Println("Error saving test item: ", err.Error())
		t.Fail()
	}

	var resultObject = testObject{}
	err = Database.BoltDb.ReadObject("test", "testItem", &resultObject)
	if err != nil {
		log.Println("Error reading test item: ", err.Error())
		t.Fail()
	}
	if resultObject.Id != "testItem" {
		log.Println("Test item not found in DB")
		t.Fail()
	}

	err = Database.BoltDb.RemoveDBFile()
	if err != nil {
		log.Println("Error removing DB file: ", err.Error())
		t.Fail()
	}
	if _, err := os.Stat(internalConfig.Database.BoltDB.FileName); !os.IsNotExist(err) {
		log.Println("DB file still exists after cleanup")
		t.Fail()
	}

}
