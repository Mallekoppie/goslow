package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"go.uber.org/zap"
)

const DefaultReadJsonRequestLimit int64 = 1024 * 1024

var (
	JsonMarshaller         jsonMarshallerOrganizer
	ErrRequestBodyTooLarge = errors.New("request body too large")
)

type jsonMarshallerOrganizer struct{}

func (j *jsonMarshallerOrganizer) ReadJsonRequestLimit(requestBody io.ReadCloser, outputType interface{}, limit int64) error {
	if requestBody == nil {
		return nil
	}
	defer requestBody.Close()
	if limit <= 0 {
		limit = DefaultReadJsonRequestLimit
	}
	data, err := io.ReadAll(io.LimitReader(requestBody, limit+1))
	if err != nil {
		if Log != nil {
			Log.Error("Error reading request body", zap.Error(err))
		}
		return err
	}
	if int64(len(data)) > limit {
		return ErrRequestBodyTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(outputType); err != nil {
		if Log != nil {
			Log.Error("Error unmarshalling response", zap.Error(err))
		}
		return err
	}
	return nil
}

func (j *jsonMarshallerOrganizer) ReadJsonRequest(requestBody io.ReadCloser, outputType interface{}) error {
	limit := DefaultReadJsonRequestLimit
	if conf, err := GetPlatformConfiguration(); err == nil && conf != nil && conf.HTTP.Server.MaxRequestBodyBytes > 0 {
		limit = conf.HTTP.Server.MaxRequestBodyBytes
	}
	return j.ReadJsonRequestLimit(requestBody, outputType, limit)
}

func (j *jsonMarshallerOrganizer) WriteJsonResponse(w http.ResponseWriter, statuscode int, response interface{}) {
	if w == nil {
		return
	}
	responseData, err := json.Marshal(response)
	if err != nil {
		if Log != nil {
			Log.Error("Unable to marshal response object", zap.Error(err))
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if statuscode != 0 {
		w.WriteHeader(statuscode)
	}
	if _, err := w.Write(responseData); err != nil && Log != nil {
		Log.Error("Unable to write response object", zap.Error(err))
	}
}
