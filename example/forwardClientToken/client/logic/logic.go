package logic

import (
	"errors"
	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
	"net/http"
)

var (
	client *http.Client
)

func init() {
	client = platform.CreateHttpClient("default")
}

func CallServer(clientToken string) error {
	token, err := platform.OAuth.GetToken("default")
	if err != nil {
		return err
	}

	request, err := http.NewRequest("GET", "http://localhost:9111/", nil)
	if err != nil {
		return err
	}

	request.Header.Add("Authorization", "Bearer "+token)
	request.Header.Add("X-Client-Token", clientToken)

	response, err := client.Do(request)
	if err != nil {
		platform.Logger.Error("Error calling server", zap.Error(err))
		return err
	}

	if response.StatusCode != http.StatusOK {
		return errors.New("incorrect response code")
	}

	return nil
}
