package platform

import (
	"errors"
	"fmt"
	"os"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"go.uber.org/zap"
)

var (
	Vault                           platformVault
	vaultClientList                 []*vaultapi.Client
	vaultEnabled                    bool
	vaultInitialized                = false
	ErrVaultNotEnabled              = errors.New("vault not enabled")
	ErrVaultUnableToReadSecrets     = errors.New("unable to read secrets from vault")
	ErrVaultNoAuthMethodsConfigured = errors.New("no auth methods configured for vault")
)

type platformVault struct {
}

// initializeVault applies the supplied configuration to the Vault subsystem.
// It is idempotent: existing state is reset before applying the new config.
func initializeVault(config *Config) error {
	vaultClientList = nil
	vaultEnabled = false
	vaultInitialized = false
	Vault = platformVault{}

	if config == nil {
		return errors.New("vault initialization requires configuration")
	}
	if !config.Vault.Enabled {
		vaultInitialized = true
		return nil
	}

	if err := setupVaultClients(config); err != nil {
		return fmt.Errorf("unable to initialize vault clients: %w", err)
	}

	vaultEnabled = true
	vaultInitialized = true
	return nil
}

func createVaultClient(address string, config *Config) (client *vaultapi.Client, err error) {
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
		Log.Error("Error creating new Vault client", zap.Error(err))
		return client, err
	}

	if config.Vault.Token.Enabled {
		if len(config.Vault.Token.TokenPath) > 0 {
			tokenValue, err := os.ReadFile(config.Vault.Token.TokenPath)
			if err != nil {
				Log.Error("Unable to read contents of token file", zap.Error(err))
			}
			client.SetToken(string(tokenValue))
		} else {
			client.SetToken(config.Vault.Token.Token)
		}
	}

	return client, nil
}

func setupVaultClients(config *Config) error {
	Log.Debug("Creating new Vault Clients")

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

	conf, confErr := GetPlatformConfiguration()
	if confErr != nil || conf == nil {
		return secrets, confErr
	}

	for _, c := range vaultClientList {
		if conf.Vault.Token.Enabled {
			secretResult, err := c.Logical().Read(path)
			if err != nil {
				Log.Error("Error retrieving secret with token", zap.Error(err), zap.String("address", c.Address()))
				continue
			}
			if secretResult != nil && secretResult.Data != nil {
				secrets = processVaultSecretResponse(secretResult.Data)
			} else {
				Log.Error("Result from retrieving secret from vault is nil")
				continue
			}

			break
		} else if conf.Vault.Cert.Enabled {
			loginSecret, err := c.Logical().Write("auth/cert/login", map[string]interface{}{})
			if err != nil {
				Log.Error("Error logging in to Vault with cert to get token", zap.Error(err))
				continue
			}
			if loginSecret == nil || loginSecret.Auth == nil || loginSecret.Auth.ClientToken == "" {
				Log.Error("Vault cert login returned no client token")
				continue
			}

			c.SetToken(loginSecret.Auth.ClientToken)

			secretResult, err := c.Logical().Read(path)
			if err != nil {
				Log.Error("Error reading secrets from Vault using cert auth method", zap.Error(err))
				continue
			}

			if secretResult != nil && secretResult.Data != nil {
				secrets = processVaultSecretResponse(secretResult.Data)
			} else {
				Log.Error("Result from retrieving secret from vault is nil")
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
