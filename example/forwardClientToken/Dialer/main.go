package main

import (
	"net/http"
	"time"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
)

func main() {
	client, err := platform.CreateHttpClient("default")
	if err != nil {
		platform.Log.Error("Error creating HTTP client", zap.Error(err))
		return
	}

	for true {
		time.Sleep(time.Second * 20)

		token, err := platform.OAuth.GetToken("default")
		if err != nil {
			platform.Log.Error("Unable to get token", zap.Error(err))
			continue
		}

		request, err := http.NewRequest("GET", "http://localhost:9112/", nil)
		if err != nil {
			platform.Log.Error("Unable to create request", zap.Error(err))
			continue
		}

		request.Header.Add("Authorization", "Bearer "+token)

		response, err := client.Do(request)
		if err != nil {
			platform.Log.Error("Error calling client service", zap.Error(err))
			continue
		}
		if response.StatusCode != http.StatusOK {
			platform.Log.Error("Incorrect response code from client service")
			continue
		}

		platform.Log.Info("Call successful")
	}
}
