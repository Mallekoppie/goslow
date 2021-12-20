package platform

import (
	"encoding/json"
	"errors"
	vaultapi "github.com/hashicorp/vault/api"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"time"
)

var (
	Vault                           platformVault
	vaultClientList                 []*vaultapi.Client
	vaultEnabled                    bool
	vaultInitialized                = false
	ErrVaultNotEnabled              = errors.New("Vault not enabled")
	ErrVaultUnableToReadSecrets     = errors.New("Unable to read secrets from Vault")
	ErrVaultNoAuthMethodsConfigured = errors.New("No auth methods configured for Vault")
)

type platformVault struct {
}

func init() {
	initializeVault()
}

func initializeVault() {
	if vaultInitialized == false {
		Vault = platformVault{}
		config, err := getPlatformConfiguration()
		if err != nil {
			Logger.Fatal("Error loading configuration in Vault initialization", zap.Error(err))
		}

		if config.Vault.Enabled {
			vaultEnabled = true
			err = setupVaultClients(config)
			if err != nil {
				Logger.Fatal("Unable to create vault client during initialization", zap.Error(err))
			}
		} else {
			vaultEnabled = false
		}

		vaultInitialized = true
	}

}

func createVaultClient(address string, config *config) (client *vaultapi.Client, err error) {
	vaultConfig := &vaultapi.Config{
		MaxRetries: config.Vault.MaxRetries,
		Timeout:    time.Second * time.Duration(config.Vault.TimeoutSeconds),
	}
	if config.Vault.IsLocalAgent {
		vaultConfig.AgentAddress = address
	} else {
		vaultConfig.Address = address
	}
	vaultTlsConfig := &vaultapi.TLSConfig{}

	if config.Vault.InsecureSkipVerify {
		vaultTlsConfig.Insecure = true
	} else {
		if len(config.Vault.CaCert) > 0 {
			vaultTlsConfig.CACert = config.Vault.CaCert
		}
	}

	if config.Vault.Cert.Enabled {
		vaultTlsConfig.ClientCert = config.Vault.Cert.CertFile
		vaultTlsConfig.ClientKey = config.Vault.Cert.KeyFile
	}

	vaultConfig.ConfigureTLS(vaultTlsConfig)
	client, err = vaultapi.NewClient(vaultConfig)
	if err != nil {
		Logger.Error("Error creating new Vault client", zap.Error(err))
		return client, err
	}

	if config.Vault.Token.Enabled {
		if len(config.Vault.Token.TokenPath) > 0 {
			tokenValue, err := ioutil.ReadFile(config.Vault.Token.TokenPath)
			if err != nil {
				Logger.Fatal("Unable to read contents of token file", zap.Error(err))
			}
			client.SetToken(string(tokenValue))
		} else {
			client.SetToken(config.Vault.Token.Token)
		}
	}

	return client, nil
}

func setupVaultClients(config *config) error {
	Logger.Debug("Creating new Vault Clients")

	vaultClientList = make([]*vaultapi.Client, 0)

	for _, address := range config.Vault.AddressList {
		client, err := createVaultClient(address, config)
		if err != nil {
			return err
		}

		vaultClientList = append(vaultClientList, client)
	}

	return nil
}

func processVaultSecretResponse(input map[string]interface{}) (secrets map[string]string) {
	secrets = make(map[string]string, 0)
	for k := range input {
		if k == "data" {
			items := input[k]
			data := items.(map[string]interface{})
			for item := range data {
				secrets[item] = data[item].(string)
			}
		}
	}

	return secrets
}

func (v *platformVault) GetSecrets(path string) (secrets map[string]string, err error) {
	if !vaultEnabled {
		return secrets, ErrVaultNotEnabled
	}

	for _, c := range vaultClientList {
		if internalConfig.Vault.Token.Enabled {
			secretResult, err := c.Logical().Read(path)
			if err != nil {
				Logger.Error("Error retrieving secret with token", zap.Error(err), zap.String("address", c.Address()))
				continue
			}
			if secretResult != nil && secretResult.Data != nil {
				secrets = processVaultSecretResponse(secretResult.Data)
			} else {
				Logger.Error("Result from retrieving secret from vault is nil")
				continue
			}

			break
		} else if internalConfig.Vault.Cert.Enabled {
			request := c.NewRequest("POST", "/v1/auth/cert/login")
			response, err := c.RawRequest(request)
			if err != nil {
				Logger.Error("Error loging in to Vault with cert to get token", zap.Error(err))
				continue
			}

			if response.StatusCode != http.StatusOK {
				Logger.Error("Incorrect responsecode from Vault when logging in with Cert", zap.Int("response_code", response.StatusCode))
				continue
			}
			defer response.Body.Close()
			responseData, err := ioutil.ReadAll(response.Body)
			if err != nil {
				Logger.Error("Error reading login response using cert to Vault", zap.Error(err))
				continue
			}

			responseModel := vaultLoginResponse{}

			err = json.Unmarshal(responseData, &responseModel)
			if err != nil {
				Logger.Error("Unable too unmarshal response from vault login", zap.Error(err))
				continue
			}

			c.SetToken(responseModel.Auth.ClientToken)

			secretResult, err := c.Logical().Read(path)
			if err != nil {
				Logger.Error("Error reading secrets from Vault using cert auth method", zap.Error(err))
				continue
			}

			if secretResult != nil && secretResult.Data != nil {
				secrets = processVaultSecretResponse(secretResult.Data)
			} else {
				Logger.Error("Result from retrieving secret from vault is nil")
				continue
			}

			break

		} else {
			return secrets, ErrVaultNoAuthMethodsConfigured
		}
	}

	if len(secrets) < 1 {
		return secrets, ErrVaultUnableToReadSecrets
	}

	return secrets, nil
}

type vaultLoginResponse struct {
	RequestID     string      `json:"request_id"`
	LeaseID       string      `json:"lease_id"`
	Renewable     bool        `json:"renewable"`
	LeaseDuration int         `json:"lease_duration"`
	Data          interface{} `json:"data"`
	WrapInfo      interface{} `json:"wrap_info"`
	Warnings      interface{} `json:"warnings"`
	Auth          struct {
		ClientToken   string   `json:"client_token"`
		Accessor      string   `json:"accessor"`
		Policies      []string `json:"policies"`
		TokenPolicies []string `json:"token_policies"`
		Metadata      struct {
			AuthorityKeyID string `json:"authority_key_id"`
			CertName       string `json:"cert_name"`
			CommonName     string `json:"common_name"`
			SerialNumber   string `json:"serial_number"`
			SubjectKeyID   string `json:"subject_key_id"`
		} `json:"metadata"`
		LeaseDuration int    `json:"lease_duration"`
		Renewable     bool   `json:"renewable"`
		EntityID      string `json:"entity_id"`
		TokenType     string `json:"token_type"`
		Orphan        bool   `json:"orphan"`
	} `json:"auth"`
}
