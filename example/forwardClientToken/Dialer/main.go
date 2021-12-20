package main

import (
	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
	"net/http"
	"time"
)

func main() {
	client := platform.CreateHttpClient("default")

	for true {
		time.Sleep(time.Second * 20)

		token, err := platform.OAuth.GetToken("default")
		if err != nil {
			platform.Logger.Error("Unable to get token", zap.Error(err))
			continue
		}

		request, err := http.NewRequest("GET", "http://localhost:9112/", nil)
		if err != nil {
			platform.Logger.Error("Unable to create request", zap.Error(err))
			continue
		}

		request.Header.Add("Authorization", "Bearer "+token)

		response, err := client.Do(request)
		if err != nil {
			platform.Logger.Error("Error calling client service", zap.Error(err))
			continue
		}
		if response.StatusCode != http.StatusOK {
			platform.Logger.Error("Incorrect response code from client service")
			continue
		}

		platform.Logger.Info("Call successful")
	}
}
