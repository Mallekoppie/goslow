package logic

import (
	"errors"
	"net/http"

	p "github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
)

var (
	client *http.Client
)

func init() {
	c, err := p.CreateHttpClient("default")
	if err != nil {
		p.Log.Error("Error creating HTTP client", zap.Error(err))
		panic(err)
	}

	client = c
}

func CallServer(clientToken string) error {
	token, err := p.OAuth.GetToken("default")
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
		p.Log.Error("Error calling server", zap.Error(err))
		return err
	}

	if response.StatusCode != http.StatusOK {
		return errors.New("incorrect response code")
	}

	return nil
}
