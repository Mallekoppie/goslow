package http

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
)

func ReadJsonRequest(requestBody io.ReadCloser, outputType interface{}) error {
	defer requestBody.Close()

	data, err := ioutil.ReadAll(requestBody)
	if err != nil {
		platform.Logger.Error("Error reading request body", zap.Error(err))
		return err
	}

	err = json.Unmarshal(data, &outputType)
	if err != nil {
		platform.Logger.Error("Error unmarshalling response", zap.Error(err))
		return err
	}

	return nil
}

func WriteJsonResponse(w http.ResponseWriter, statuscode int, response interface{}) {
	responseData, err := json.Marshal(response)
	if err != nil {
		platform.Logger.Error("Unable to marshal response object", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(statuscode)
	w.Write(responseData)
}
