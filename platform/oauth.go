package platform

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
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
	oAuthTokens                    map[string]string

	OAuth oAuthOrganiser
)

const (
	oAuthMaxIdleConnections int = 20
	oAuthRequestTimeout     int = 30
)

type oAuthOrganiser struct{}

func init() {
	initializeVault()
	initializeOAuthTokenClients()

	oAuthHttpClient = createOAuthHTTPClient()
}

func initializeOAuthTokenClients() {
	if len(internalConfig.Auth.Client.OAuth) > 0 {
		oAuthTokens = make(map[string]string, 0)
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
			}

			oAuthClientTokenConfiguration = append(oAuthClientTokenConfiguration, v)

			go autoRenewOAuth2Token(v)
		}
	}
}

func autoRenewOAuth2Token(config clientTokenConfig) {
	currentToken := ""
	// Always renew
	currentExpiryTime := time.Now().Add(-(time.Minute * 60))
	for true {
		if time.Since(currentExpiryTime).Minutes() > -(config.RenewCheckTimeMinutes) {
			Logger.Info("Renewing token", zap.String("config_id", config.ID))
			auth2Token, err := internalGetOAuth2Token(config.ID)
			if err != nil {
				Logger.Error("Error getting token. Waiting before retrying", zap.Error(err))
				time.Sleep(time.Second * 5)
				continue
			}
			currentToken = auth2Token
			currentExpiryTime = getExpiryTimeFromToken(currentToken)
			oAuthTokens[config.ID] = currentToken

			Logger.Info("Token renewed", zap.String("config_id", config.ID), zap.Time("next_expiry", currentExpiryTime))
		}

		time.Sleep(time.Duration(config.RenewCheckIntervalSeconds) * time.Second)
	}
}

func getExpiryTimeFromToken(data string) time.Time {
	// This is probably bad...
	token, _ := jwt.Parse(data, func(token *jwt.Token) (interface{}, error) {
		return token, nil
	})

	claims := token.Claims.(jwt.MapClaims)

	exp := claims["exp"].(float64)
	result := time.Unix(int64(exp), 0)

	return result
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

func (o oAuthOrganiser) GetToken(id string) (accessToken string, err error) {
	token := oAuthTokens[id]

	if len(token) < 1 {
		Logger.Error("No token in local cache", zap.String("config_id", id))
		auth2Token, err := internalGetOAuth2Token(id)
		if err != nil {
			Logger.Error("Unable to retrieve OAuth2 token from IDP", zap.String("config_id", id), zap.Error(err))
			return "", err
		}

		return auth2Token, nil
	}

	return token, nil
}

func internalGetOAuth2Token(id string) (accessToken string, err error) {
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
	if err != nil {
		Logger.Error("Error getting OAuth2 token from IDP", zap.String("config_id", id),
			zap.Error(err))
		return "", err
	}
	defer response.Body.Close()
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
