package platform

import (
	"crypto/tls"
	"errors"
	"go.uber.org/zap"
	"net/http"
	"time"
)

var (
	ErrHttpClientConfigNotFound = errors.New("http client config not found")
)

func CreateHttpClient(id string) *http.Client {
	var clientConfig httpClientConfig
	for _, v := range internalConfig.HTTP.Clients {
		if v.ID == id {
			clientConfig = v
			break
		}
	}

	if len(clientConfig.ID) < 1 {
		Logger.Fatal("No http client configuration found", zap.String("config_id", id))
	}

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: clientConfig.MaxIdleConnections,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: clientConfig.TLSVerify},
		},
		Timeout: time.Duration(clientConfig.RequestTimeout) * time.Second,
	}

	return client
}
