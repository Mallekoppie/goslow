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

	Logger.Debug("Calling SaveObject from test")
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
