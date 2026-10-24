package platform

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

var (
	oAuthClientTokenConfiguration  []clientTokenConfig
	ErrOAuthClientConfigNotFound   = errors.New("oauth client config not found")
	ErrOAuthIncorrectIDPStatusCode = errors.New("incorrect status code on token request to idp")
	ErrOAuthInvalidEndpoint        = errors.New("invalid oauth token endpoint")
	oAuthHttpClient                *http.Client
	oAuthClientOnce                sync.Once
	oAuthDefaultTLSConfig          = &tls.Config{}
	oAuthTokens                    map[string]string
	oAuthStateLock                 sync.RWMutex
	oAuthRenewStop                 chan struct{}
	oAuthRenewWG                   sync.WaitGroup

	OAuth oAuthOrganiser
)

const (
	oAuthMaxIdleConnections int = 20
	oAuthRequestTimeout     int = 30
)

type oAuthOrganiser struct{}

func ensureOAuthHTTPClient() *http.Client {
	oAuthClientOnce.Do(func() {
		oAuthHttpClient = createOAuthHTTPClient()
	})
	return oAuthHttpClient
}

// createInsecureOAuthHTTPClient returns a client with certificate verification
// disabled. It is only used for clients that explicitly opt in.
func createInsecureOAuthHTTPClient() *http.Client {
	client := createOAuthHTTPClient()
	client.Transport = &http.Transport{
		MaxIdleConnsPerHost: oAuthMaxIdleConnections,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	}
	return client
}

// initializeOAuthTokenClients applies config-driven OAuth client setup. It stops
// existing renewal workers, resets caches, and skips clients that cannot be
// configured instead of enabling them with empty credentials.
func initializeOAuthTokenClients(conf *Config) error {
	stopOAuthTokenRenewal()
	ensureOAuthHTTPClient()

	oAuthStateLock.Lock()
	oAuthTokens = nil
	oAuthClientTokenConfiguration = nil
	oAuthRenewStop = nil
	oAuthStateLock.Unlock()

	if conf == nil || len(conf.Auth.Client.OAuth) == 0 {
		return nil
	}

	oAuthStateLock.Lock()
	oAuthTokens = make(map[string]string, len(conf.Auth.Client.OAuth))
	oAuthClientTokenConfiguration = make([]clientTokenConfig, 0, len(conf.Auth.Client.OAuth))
	oAuthRenewStop = make(chan struct{})
	stop := oAuthRenewStop
	oAuthStateLock.Unlock()

	for _, v := range conf.Auth.Client.OAuth {
		if conf.Vault.Enabled && len(v.VaultPath) > 0 {
			secrets, err := Vault.GetSecrets(v.VaultPath)
			if err != nil {
				Log.Error("Unable to retrieve OAuth client configs from Vault; skipping client",
					zap.String("vault_path", v.VaultPath),
					zap.String("config_id", v.ID),
					zap.Error(err))
				continue
			}

			v.vaultClientIdValue = secrets[v.VaultClientIdKey]
			v.vaultClientSecretValue = secrets[v.VaultClientSecretKey]
			v.vaultUsernameValue = secrets[v.VaultUsernameKey]
			v.vaultPasswordValue = secrets[v.VaultPasswordKey]
			v.vaultIdpTokenEndpointValue = secrets[v.VaultIdpTokenEndpointKey]
			v.vaultEnabled = true

			if len(v.vaultClientIdValue) == 0 || len(v.vaultIdpTokenEndpointValue) == 0 {
				Log.Error("OAuth client config from Vault is incomplete; skipping client", zap.String("config_id", v.ID))
				continue
			}
		}

		oAuthStateLock.Lock()
		oAuthClientTokenConfiguration = append(oAuthClientTokenConfiguration, v)
		oAuthStateLock.Unlock()

		// Do not start a renewal loop that can only fail: without an endpoint
		// there is nothing to call. GetToken still reports the misconfiguration
		// when a caller actually asks for a token.
		hasEndpoint := len(v.IdpTokenEndpoint) > 0 || (v.vaultEnabled && len(v.vaultIdpTokenEndpointValue) > 0)
		if !hasEndpoint {
			Log.Warn("Skipping OAuth token auto-renewal: no token endpoint configured", zap.String("config_id", v.ID))
			continue
		}

		oAuthRenewWG.Add(1)
		go func(cfg clientTokenConfig, stop <-chan struct{}) {
			defer oAuthRenewWG.Done()
			autoRenewOAuth2Token(cfg, stop)
		}(v, stop)
	}

	return nil
}

// stopOAuthTokenRenewal stops all token renewal goroutines and waits for them to exit.
func stopOAuthTokenRenewal() {
	oAuthStateLock.Lock()
	if oAuthRenewStop != nil {
		close(oAuthRenewStop)
		oAuthRenewStop = nil
	}
	oAuthStateLock.Unlock()

	oAuthRenewWG.Wait()
}

// waitOrStop returns false when the renewal loop was asked to stop.
func waitOrStop(stop <-chan struct{}, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-stop:
		return false
	case <-timer.C:
		return true
	}
}

func autoRenewOAuth2Token(config clientTokenConfig, stop <-chan struct{}) {
	// Always renew on start
	currentExpiryTime := time.Now().Add(-(time.Minute * 60))

	checkMinutes := config.RenewCheckTimeMinutes
	if checkMinutes <= 0 {
		checkMinutes = 5
	}
	interval := time.Duration(config.RenewCheckIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}

	for {
		if time.Since(currentExpiryTime).Minutes() > -checkMinutes {
			Log.Info("Renewing token", zap.String("config_id", config.ID))
			auth2Token, err := internalGetOAuth2Token(config.ID)
			if err != nil {
				Log.Error("Error getting token. Waiting before retrying", zap.Error(err))
				if !waitOrStop(stop, 5*time.Second) {
					return
				}
				continue
			}

			currentExpiryTime = getExpiryTimeFromToken(auth2Token)
			if currentExpiryTime.IsZero() {
				currentExpiryTime = time.Now().Add(time.Duration(checkMinutes) * time.Minute)
			}

			oAuthStateLock.Lock()
			if oAuthTokens == nil {
				oAuthTokens = make(map[string]string)
			}
			oAuthTokens[config.ID] = auth2Token
			oAuthStateLock.Unlock()

			Log.Info("Token renewed", zap.String("config_id", config.ID), zap.Time("next_expiry", currentExpiryTime))
		}

		if !waitOrStop(stop, interval) {
			return
		}
	}
}

func getExpiryTimeFromToken(data string) time.Time {
	// Tokens from some identity providers are opaque, so failure here is not fatal.
	// Claims are read without verifying the signature: the token has already been
	// fetched from the IDP by the time this runs.
	parser := &jwt.Parser{}
	token, _, err := parser.ParseUnverified(data, jwt.MapClaims{})
	if err != nil || token == nil {
		return time.Time{}
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return time.Time{}
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return time.Time{}
	}

	return time.Unix(int64(exp), 0)
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
	oAuthStateLock.RLock()
	token := oAuthTokens[id]
	oAuthStateLock.RUnlock()

	if len(token) < 1 {
		Log.Error("No token in local cache", zap.String("config_id", id))
		auth2Token, err := internalGetOAuth2Token(id)
		if err != nil {
			Log.Error("Unable to retrieve OAuth2 token from IDP", zap.String("config_id", id), zap.Error(err))
			return "", err
		}

		return auth2Token, nil
	}

	return token, nil
}

func internalGetOAuth2Token(id string) (accessToken string, err error) {
	var clientConfig clientTokenConfig
	oAuthStateLock.RLock()
	for _, v := range oAuthClientTokenConfiguration {
		if v.ID == id {
			clientConfig = v
			break
		}
	}
	oAuthStateLock.RUnlock()

	if len(clientConfig.ID) < 1 {
		Log.Error("Found no OAuth2 client config", zap.String("config_id", id))
		return "", ErrOAuthClientConfigNotFound
	}

	var endpoint string
	var payload *strings.Reader
	if clientConfig.vaultEnabled {
		endpoint = clientConfig.vaultIdpTokenEndpointValue

		payload = strings.NewReader(fmt.Sprintf("client_id=%s&client_secret=%s&username=%s&password=%s&grant_type=password",
			clientConfig.vaultClientIdValue,
			clientConfig.vaultClientSecretValue,
			clientConfig.vaultUsernameValue,
			clientConfig.vaultPasswordValue))
	} else {
		endpoint = clientConfig.IdpTokenEndpoint

		payload = strings.NewReader(fmt.Sprintf("client_id=%s&client_secret=%s&username=%s&password=%s&grant_type=password",
			clientConfig.ClientID,
			clientConfig.ClientSecret,
			clientConfig.Username,
			clientConfig.Password))
	}

	parsedEndpoint, parseErr := url.Parse(endpoint)
	if parseErr != nil || parsedEndpoint.Host == "" ||
		(parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") {
		Log.Error("OAuth token endpoint is invalid", zap.String("config_id", id))
		return "", fmt.Errorf("%w: %q", ErrOAuthInvalidEndpoint, endpoint)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(oAuthRequestTimeout)*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		Log.Error("Unable to build OAuth2 token request", zap.String("config_id", id), zap.Error(err))
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := ensureOAuthHTTPClient()
	if clientConfig.InsecureSkipVerify {
		client = createInsecureOAuthHTTPClient()
	}

	response, err := client.Do(request)
	if err != nil {
		Log.Error("Error getting OAuth2 token from IDP", zap.String("config_id", id),
			zap.Error(err))
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		Log.Error("Error reading response body from IDP", zap.String("config_id", id),
			zap.Error(err))

		return "", err
	}

	if response.StatusCode != http.StatusOK {
		Log.Error("Received incorrect http response code when requesting token from IDP",
			zap.String("config_id", id))

		return "", ErrOAuthIncorrectIDPStatusCode
	}

	oauthResponse := idpOAuth2TokenResponse{}

	err = json.Unmarshal(body, &oauthResponse)
	if err != nil {
		Log.Error("Error unmarshalling response from IDP", zap.String("config_id", id),
			zap.Error(err))

		return "", err
	}

	return oauthResponse.AccessToken, nil
}

// ValidateClientToken verifies a forwarded client token with the same verifier
// used for route authorization: signature, expiry, and issuer are checked, and
// provider keys are refreshed when the token presents an unknown key ID.
//
// Audience is the one explicit difference. A forwarded client token was minted
// for a different client, so its audience is not compared with this service's
// configured client ID. Requiring it would reject every forwarded token.
func (o oAuthOrganiser) ValidateClientToken(ctx context.Context, rawToken string) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conf, err := GetPlatformConfiguration()
	if err != nil {
		return nil, err
	}
	settings := AuthSettingsFromConfig(conf).OAuth
	if settings.IdpWellKnownURL == "" {
		return nil, errors.New("client token verification requires oauth.idpwellknownurl")
	}

	verifier, err := newOIDCClaimsVerifier(ctx, settings, acceptAnyAudience)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	return verifier.Verify(ctx, rawToken)
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
