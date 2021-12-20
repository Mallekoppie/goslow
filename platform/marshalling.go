package platform

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"go.uber.org/zap"
)

var (
	JsonMarshaller jsonMarshallerOrganizer
)

func init() {
	JsonMarshaller = jsonMarshallerOrganizer{}
}

// This is just to make the platform interface nice
type jsonMarshallerOrganizer struct {
}

func (j *jsonMarshallerOrganizer) ReadJsonRequest(requestBody io.ReadCloser, outputType interface{}) error {
	defer requestBody.Close()

	data, err := ioutil.ReadAll(requestBody)
	if err != nil {
		Logger.Error("Error reading request body", zap.Error(err))
		return err
	}

	err = json.Unmarshal(data, &outputType)
	if err != nil {
		Logger.Error("Error unmarshalling response", zap.Error(err))
		return err
	}

	return nil
}

func (j *jsonMarshallerOrganizer) WriteJsonResponse(w http.ResponseWriter, statuscode int, response interface{}) {
	responseData, err := json.Marshal(response)
	if err != nil {
		Logger.Error("Unable to marshal response object", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(statuscode)
	w.Write(responseData)
}
