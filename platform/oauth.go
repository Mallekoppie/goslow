package platform

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

var (
	oAuthClientTokenEnabled        = false
	oAuthClientTokenConfiguration  []clientTokenConfig
	ErrOAuthClientConfigNotFound   = errors.New("oauth client config not found")
	ErrOAuthIncorrectIDPStatusCode = errors.New("incorrect status code on token request to idp")
	oAuthHttpClient                *http.Client
	oAuthDefaultTLSConfig          = &tls.Config{InsecureSkipVerify: true}
)

const (
	oAuthMaxIdleConnections int = 20
	oAuthRequestTimeout     int = 30
)

func init() {
	initializeVault()
	initializeOAuthTokenClients()

	oAuthHttpClient = createOAuthHTTPClient()
}

func initializeOAuthTokenClients() {
	if len(internalConfig.Auth.Client.OAuth) > 0 {
		oAuthClientTokenConfiguration = make([]clientTokenConfig, 0)
		oAuthClientTokenEnabled = true

		for _, v := range internalConfig.Auth.Client.OAuth {
			if internalConfig.Vault.Enabled && len(v.VaultPath) > 0 {
				secrets, err := Vault.GetSecrets(v.VaultPath)
				if err != nil {
					Logger.Fatal("Unable to retrieve OAuth client configs from Vault", zap.String("vault_path", v.VaultPath),
						zap.String("config_id", v.ID),
						zap.Error(err))
				}

				v.vaultClientIdValue = secrets[v.VaultClientIdKey]
				v.vaultClientSecretValue = secrets[v.VaultClientSecretKey]
				v.vaultUsernameValue = secrets[v.VaultUsernameKey]
				v.vaultPasswordValue = secrets[v.VaultPasswordKey]
				v.vaultIdpTokenEndpointValue = secrets[v.VaultIdpTokenEndpointKey]
				v.vaultEnabled = true

				oAuthClientTokenConfiguration = append(oAuthClientTokenConfiguration, v)
			}
		}
	}
}

func createOAuthHTTPClient() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: oAuthMaxIdleConnections,
			TLSClientConfig:     oAuthDefaultTLSConfig,
		},
		Timeout: time.Duration(oAuthRequestTimeout) * time.Second,
	}

	return client
}

func GetToken(id string) (accessToken string, err error) {
	var clientConfig clientTokenConfig
	for _, v := range oAuthClientTokenConfiguration {
		if v.ID == id {
			clientConfig = v
			break
		}
	}

	if len(clientConfig.ID) < 1 {
		Logger.Error("Found no OAuth2 client config", zap.String("config_id", id))
	}

	var url string
	var payload *strings.Reader
	if clientConfig.vaultEnabled {
		url = clientConfig.vaultIdpTokenEndpointValue

		payload = strings.NewReader(fmt.Sprintf("client_id=%s&client_secret=%s&username=%s&password=%s&grant_type=password",
			clientConfig.vaultClientIdValue,
			clientConfig.vaultClientSecretValue,
			clientConfig.vaultUsernameValue,
			clientConfig.vaultPasswordValue))
	} else {
		url = clientConfig.IdpTokenEndpoint

		payload = strings.NewReader(fmt.Sprintf("client_id=%s&client_secret=%s&username=%s&password=%s&grant_type=password",
			clientConfig.ClientID,
			clientConfig.ClientSecret,
			clientConfig.Username,
			clientConfig.Password))
	}

	request, _ := http.NewRequest("POST", url, payload)

	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	response, err := oAuthHttpClient.Do(request)
	defer response.Body.Close()
	if err != nil {
		Logger.Error("Error getting OAuth2 token from IDP", zap.String("config_id", id),
			zap.Error(err))
		return "", err
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		Logger.Error("Error reading response body from IDP", zap.String("config_id", id),
			zap.Error(err))

		return "", err
	}

	if response.StatusCode != http.StatusOK {
		Logger.Error("Received incorrect http response code when requesting token from IDP",
			zap.String("config_id", id))

		return "", ErrOAuthIncorrectIDPStatusCode
	}

	oauthResponse := idpOAuth2TokenResponse{}

	err = json.Unmarshal(body, &oauthResponse)
	if err != nil {
		Logger.Error("Error unmarshalling response from IDP", zap.String("config_id", id),
			zap.Error(err))

		return "", err
	}

	return oauthResponse.AccessToken, nil
}

type idpOAuth2TokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	NotBeforePolicy  int    `json:"not-before-policy"`
	SessionState     string `json:"session_state"`
	Scope            string `json:"scope"`
}
