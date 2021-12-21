package platform

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/lestrrat/go-jwx/jwk"
	"go.uber.org/zap"
	"io"
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

	issuerJwkUrlMap map[string]idpDetailsCacheItem
)

const (
	oAuthMaxIdleConnections int = 20
	oAuthRequestTimeout     int = 30
)

type oAuthOrganiser struct{}

type idpDetailsCacheItem struct {
	JwkUrl       string
	KeySet       *jwk.Set
	SetPopulated bool
}

func init() {
	initializeVault()
	initializeOAuthTokenClients()

	oAuthHttpClient = createOAuthHTTPClient()

	issuerJwkUrlMap = make(map[string]idpDetailsCacheItem, 0)
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

func ValidateClientToken(rawToken string) (parsedToken *jwt.Token, err error) {
	token, _ := jwt.Parse(rawToken, func(token *jwt.Token) (interface{}, error) {
		return token, nil
	})

	claims := token.Claims.(jwt.MapClaims)
	issuer := claims["iss"].(string)

	jwksUrl, ok := issuerJwkUrlMap[issuer]
	if ok == false {
		jwksUrl = idpDetailsCacheItem{SetPopulated: false}
		jwksUrl.JwkUrl, err = getJksUrl(issuer)
		if err != nil {
			// Error already logged. Just stop execution
			return
		}

		// Cache it
		issuerJwkUrlMap[issuer] = jwksUrl
	}

	parsedToken, err = jwt.Parse(rawToken, func(token *jwt.Token) (interface{}, error) {
		if jwksUrl.SetPopulated == false {
			set, err := jwk.FetchHTTP(jwksUrl.JwkUrl)
			if err != nil {
				return nil, err
			}

			jwksUrl.KeySet = set
		}

		keyID, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("expecting JWT header to have string kid")
		}

		if key := jwksUrl.KeySet.LookupKeyID(keyID); len(key) == 1 {
			return key[0].Materialize()
		}

		return nil, fmt.Errorf("unable to find key %q", keyID)
	})
	if err != nil {
		Logger.Error("Token Validation failed", zap.Error(err))
		return parsedToken, err
	}

	return parsedToken, nil
}

func getJksUrl(issuerUrl string) (url string, err error) {
	wellKnownExtension := "/.well-known/openid-configuration"
	fullPath := fmt.Sprintf("%s%s", issuerUrl, wellKnownExtension)

	response, err := oAuthHttpClient.Get(fullPath)
	if err != nil {
		Logger.Error("Unable to call IDP Well Known configuration endpoint", zap.String("idp_issuer", issuerUrl),
			zap.Error(err))
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		Logger.Error("Incorrect response code from IDP Well Known url", zap.Int("status_code_expected", 200),
			zap.Int("status_code_actual", response.StatusCode), zap.String("idp_issuer", issuerUrl))
		return "", errors.New("incorrect response code. expected 200")
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		Logger.Error("Error reading response body from IDP Well Known config", zap.String("idp_issuer", issuerUrl),
			zap.Error(err))
		return "", err
	}

	responseBody := IDPWellKnownConfiguration{}

	err = json.Unmarshal(responseData, &responseBody)
	if err != nil {
		Logger.Error("Error unmarshalling IDP Well Known response body", zap.String("idp_issuer", issuerUrl),
			zap.Error(err))
		return "", err
	}

	return responseBody.JwksURI, nil
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

type IDPWellKnownConfiguration struct {
	Issuer                                                    string   `json:"issuer"`
	AuthorizationEndpoint                                     string   `json:"authorization_endpoint"`
	TokenEndpoint                                             string   `json:"token_endpoint"`
	IntrospectionEndpoint                                     string   `json:"introspection_endpoint"`
	UserinfoEndpoint                                          string   `json:"userinfo_endpoint"`
	EndSessionEndpoint                                        string   `json:"end_session_endpoint"`
	JwksURI                                                   string   `json:"jwks_uri"`
	CheckSessionIframe                                        string   `json:"check_session_iframe"`
	GrantTypesSupported                                       []string `json:"grant_types_supported"`
	ResponseTypesSupported                                    []string `json:"response_types_supported"`
	SubjectTypesSupported                                     []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported                          []string `json:"id_token_signing_alg_values_supported"`
	IDTokenEncryptionAlgValuesSupported                       []string `json:"id_token_encryption_alg_values_supported"`
	IDTokenEncryptionEncValuesSupported                       []string `json:"id_token_encryption_enc_values_supported"`
	UserinfoSigningAlgValuesSupported                         []string `json:"userinfo_signing_alg_values_supported"`
	RequestObjectSigningAlgValuesSupported                    []string `json:"request_object_signing_alg_values_supported"`
	RequestObjectEncryptionAlgValuesSupported                 []string `json:"request_object_encryption_alg_values_supported"`
	RequestObjectEncryptionEncValuesSupported                 []string `json:"request_object_encryption_enc_values_supported"`
	ResponseModesSupported                                    []string `json:"response_modes_supported"`
	RegistrationEndpoint                                      string   `json:"registration_endpoint"`
	TokenEndpointAuthMethodsSupported                         []string `json:"token_endpoint_auth_methods_supported"`
	TokenEndpointAuthSigningAlgValuesSupported                []string `json:"token_endpoint_auth_signing_alg_values_supported"`
	IntrospectionEndpointAuthMethodsSupported                 []string `json:"introspection_endpoint_auth_methods_supported"`
	IntrospectionEndpointAuthSigningAlgValuesSupported        []string `json:"introspection_endpoint_auth_signing_alg_values_supported"`
	AuthorizationSigningAlgValuesSupported                    []string `json:"authorization_signing_alg_values_supported"`
	AuthorizationEncryptionAlgValuesSupported                 []string `json:"authorization_encryption_alg_values_supported"`
	AuthorizationEncryptionEncValuesSupported                 []string `json:"authorization_encryption_enc_values_supported"`
	ClaimsSupported                                           []string `json:"claims_supported"`
	ClaimTypesSupported                                       []string `json:"claim_types_supported"`
	ClaimsParameterSupported                                  bool     `json:"claims_parameter_supported"`
	ScopesSupported                                           []string `json:"scopes_supported"`
	RequestParameterSupported                                 bool     `json:"request_parameter_supported"`
	RequestURIParameterSupported                              bool     `json:"request_uri_parameter_supported"`
	RequireRequestURIRegistration                             bool     `json:"require_request_uri_registration"`
	CodeChallengeMethodsSupported                             []string `json:"code_challenge_methods_supported"`
	TLSClientCertificateBoundAccessTokens                     bool     `json:"tls_client_certificate_bound_access_tokens"`
	RevocationEndpoint                                        string   `json:"revocation_endpoint"`
	RevocationEndpointAuthMethodsSupported                    []string `json:"revocation_endpoint_auth_methods_supported"`
	RevocationEndpointAuthSigningAlgValuesSupported           []string `json:"revocation_endpoint_auth_signing_alg_values_supported"`
	BackchannelLogoutSupported                                bool     `json:"backchannel_logout_supported"`
	BackchannelLogoutSessionSupported                         bool     `json:"backchannel_logout_session_supported"`
	DeviceAuthorizationEndpoint                               string   `json:"device_authorization_endpoint"`
	BackchannelTokenDeliveryModesSupported                    []string `json:"backchannel_token_delivery_modes_supported"`
	BackchannelAuthenticationEndpoint                         string   `json:"backchannel_authentication_endpoint"`
	BackchannelAuthenticationRequestSigningAlgValuesSupported []string `json:"backchannel_authentication_request_signing_alg_values_supported"`
	RequirePushedAuthorizationRequests                        bool     `json:"require_pushed_authorization_requests"`
	PushedAuthorizationRequestEndpoint                        string   `json:"pushed_authorization_request_endpoint"`
	MtlsEndpointAliases                                       struct {
		TokenEndpoint                      string `json:"token_endpoint"`
		RevocationEndpoint                 string `json:"revocation_endpoint"`
		IntrospectionEndpoint              string `json:"introspection_endpoint"`
		DeviceAuthorizationEndpoint        string `json:"device_authorization_endpoint"`
		RegistrationEndpoint               string `json:"registration_endpoint"`
		UserinfoEndpoint                   string `json:"userinfo_endpoint"`
		PushedAuthorizationRequestEndpoint string `json:"pushed_authorization_request_endpoint"`
		BackchannelAuthenticationEndpoint  string `json:"backchannel_authentication_endpoint"`
	} `json:"mtls_endpoint_aliases"`
}
