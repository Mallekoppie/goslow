package platform

import (
	"errors"
	vaultapi "github.com/hashicorp/vault/api"
	"go.uber.org/zap"
	"io/ioutil"
	"time"
)

var (
	Vault                           PlatformVault
	vaultClientList                 []*vaultapi.Client
	vaultEnabled                    bool
	ErrVaultNotEnabled              = errors.New("Vault not enabled")
	ErrVaultUnableToReadSecrets     = errors.New("Unable to read secrets from Vault")
	ErrVaultNoAuthMethodsConfigured = errors.New("No auth methods configured for Vault")
)

type PlatformVault struct {
}

func init() {
	Vault = PlatformVault{}
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

		if config.Vault.Cert.Enabled {
			vaultTlsConfig.ClientCert = config.Vault.Cert.CertFile
			vaultTlsConfig.ClientKey = config.Vault.Cert.KeyFile
		}
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

func (v *PlatformVault) GetSecrets(path string) (secrets map[string]interface{}, err error) {
	if !vaultEnabled {
		return secrets, ErrVaultNotEnabled
	}

	for _, c := range vaultClientList {
		if internalConfig.Vault.Token.Enabled {
			result, err := c.Logical().Read(path)
			if err != nil {
				Logger.Error("Error retrieving secret with token", zap.Error(err), zap.String("address", c.Address()))
				continue
			}
			if result.Data != nil {
				secrets = result.Data
			} else {
				Logger.Error("Result from retrieving secret from vault is nil")
				continue
			}

			break
		} else if internalConfig.Vault.Cert.Enabled {
			result, err := c.Logical().Write("/v1/auth/cert/login", nil)
			if err != nil {
				Logger.Error("Error loging in to Vault with cert to get token")
				continue
			}

			id, err := result.TokenID()
			if err != nil {
				Logger.Error("Error getting token from cert login to Vault", zap.Error(err))
				continue
			}
			c.SetToken(id)

			secretResult, err := c.Logical().Read(path)
			if err != nil {
				Logger.Error("Error reading secrets from Vault using cert auth method", zap.Error(err))
				continue
			}

			if secretResult != nil {
				secrets = secretResult.Data
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
