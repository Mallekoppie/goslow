package service

import (
	"net/http"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
)

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	platform.Logger.Info("We arrived at a new world!!!!")

	w.Write([]byte("Hello World"))
}

func WriteObject(w http.ResponseWriter, r *http.Request) {
	platform.Logger.Info("Writing object")

	testobject := DBTestObject{}

	err := platform.JsonMarshaller.ReadJsonRequest(r.Body, &testobject)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	platform.Logger.Info("Incoming object id", zap.String("id", testobject.Id))

	err = platform.Database.BoltDb.SaveObject("test", testobject.Id, testobject)
	if err != nil {
		platform.Logger.Error("Error saving object", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func ReadObject(w http.ResponseWriter, r *http.Request) {
	platform.Logger.Info("Writing object")

	testobject := DBTestObject{}

	err := platform.JsonMarshaller.ReadJsonRequest(r.Body, &testobject)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	resultObject := DBTestObject{}
	err = platform.Database.BoltDb.ReadObject("test", testobject.Id, &resultObject)
	if err != nil {
		platform.Logger.Error("Error reading objet", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	platform.JsonMarshaller.WriteJsonResponse(w, 200, resultObject)
}

type DBTestObject struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Surname string `json:"surname"`
}
