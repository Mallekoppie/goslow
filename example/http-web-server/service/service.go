package service

import (
	"bytes"
	"http-web-server/model"
	"net/http"
	"slices"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
)

// OAuthRequiredRole is the role the /oauth2 route demands. It is shared with
// the route declaration so the gate and the handler cannot drift.
const OAuthRequiredRole = "read"

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	platform.Log.Info("We arrived at a new world!!!!")

	w.Write([]byte("Hello World"))
}

func WriteObject(w http.ResponseWriter, r *http.Request) {
	platform.Log.Info("Writing object")

	testobject := DBTestObject{}

	err := platform.JsonMarshaller.ReadJsonRequest(r.Body, &testobject)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	platform.Log.Info("Incoming object id", zap.String("id", testobject.Id))

	err = platform.Database.BoltDb.SaveObject("test", testobject.Id, testobject)
	if err != nil {
		platform.Log.Error("Error saving object", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func ReadObject(w http.ResponseWriter, r *http.Request) {
	platform.Log.Info("Writing object")

	testobject := DBTestObject{}

	err := platform.JsonMarshaller.ReadJsonRequest(r.Body, &testobject)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	resultObject := DBTestObject{}
	err = platform.Database.BoltDb.ReadObject("test", testobject.Id, &resultObject)
	if err != nil {
		if err == platform.ErrNoEntryFoundInDB {
			w.WriteHeader(http.StatusNoContent)
			return
		} else {
			platform.Log.Error("Error reading objet", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

	}

	platform.JsonMarshaller.WriteJsonResponse(w, http.StatusOK, resultObject)
}

type DBTestObject struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Surname string `json:"surname"`
}

func GetConfiguration(w http.ResponseWriter, r *http.Request) {
	conf := model.Config{}

	err := platform.GetComponentConfiguration("componentconfigexample", &conf)
	if err != nil {
		platform.Log.Error("Error reading component configuration", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	platform.JsonMarshaller.WriteJsonResponse(w, http.StatusOK, conf)
}

func ReadAll(w http.ResponseWriter, r *http.Request) {

	results, err := platform.Database.BoltDb.ReadAllObjects("test")
	if err != nil {
		platform.Log.Error("Error getting all objects", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data := bytes.NewBufferString("")
	for _, v := range results {
		data.Write([]byte(v))
	}

	w.WriteHeader(http.StatusOK)
	w.Write(data.Bytes())

}

func GetSecrets(w http.ResponseWriter, r *http.Request) {

	secrets, err := platform.Vault.GetSecrets("kv-v2/data/dev/test/creds")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}

	response := GetSecretResponse{
		Username: secrets["username"],
		Password: secrets["password"],
	}

	platform.JsonMarshaller.WriteJsonResponse(w, 200, response)
}

func GetOAuthDetails(w http.ResponseWriter, r *http.Request) {
	identity, ok := platform.IdentityFromContext(r.Context())
	if !ok {
		http.Error(w, "No identity found for the caller", http.StatusUnauthorized)
		return
	}

	if !slices.Contains(identity.Roles, OAuthRequiredRole) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	platform.Log.Info("OAuth details", zap.String("role_1", identity.Roles[0]), zap.String("username", identity.Username),
		zap.String("idp_ref", identity.Issuer))

	platform.JsonMarshaller.WriteJsonResponse(w, http.StatusOK, OAuthCallerDetails{
		Username: identity.Username,
		Issuer:   identity.Issuer,
		Roles:    identity.Roles,
	})
}

// OAuthCallerDetails describes the verified caller without exposing the raw
// credential or the complete claims set.
type OAuthCallerDetails struct {
	Username string   `json:"username"`
	Issuer   string   `json:"issuer"`
	Roles    []string `json:"roles"`
}

type GetSecretResponse struct {
	Username string
	Password string
}
