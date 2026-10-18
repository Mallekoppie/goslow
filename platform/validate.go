package platform

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
)

// ValidateServingConfiguration performs checks that require the process to be
// starting a server: TLS material, listener addresses, and enabled auth
// features. Configuration parsing stays free of filesystem checks.
func ValidateServingConfiguration(conf *Config) error {
	if conf == nil {
		return errors.New("config is nil")
	}
	if err := validateConfiguration(conf); err != nil {
		return err
	}

	if conf.HTTP.Server.TLSEnabled {
		if err := validateTLSFiles("http", conf.HTTP.Server.TLSCertFileName, conf.HTTP.Server.TLSKeyFileName); err != nil {
			return err
		}
	}
	if conf.Grpc.Server.TLSEnabled {
		if err := validateTLSFiles("grpc", conf.Grpc.Server.TLSCertFileName, conf.Grpc.Server.TLSKeyFileName); err != nil {
			return err
		}
	}

	if address := conf.HTTP.Server.ListeningAddress; address != "" {
		if err := validateListenAddress("http.server.listeningaddress", address); err != nil {
			return err
		}
	}
	if address := conf.Grpc.Server.ListeningAddress; address != "" {
		if err := validateListenAddress("grpc.server.listeningaddress", address); err != nil {
			return err
		}
	}

	if conf.Auth.Server.OAuth.Enabled {
		if conf.Auth.Server.OAuth.IdpWellKnownURL == "" {
			return errors.New("auth.server.oauth.idpwellknownurl is required when oauth is enabled")
		}
		parsed, err := url.Parse(conf.Auth.Server.OAuth.IdpWellKnownURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("auth.server.oauth.idpwellknownurl is invalid: %q", conf.Auth.Server.OAuth.IdpWellKnownURL)
		}
		if conf.Auth.Server.OAuth.ClientID == "" {
			return errors.New("auth.server.oauth.clientid is required when oauth is enabled")
		}
		if len(conf.Auth.Server.OAuth.AllowedAlgorithms) == 0 {
			return errors.New("auth.server.oauth.allowedalgorithms is required when oauth is enabled")
		}
	}

	if conf.Auth.Server.LocalJwt.Enabled {
		if conf.Auth.Server.LocalJwt.JwtSigningKey == "" {
			return errors.New("auth.server.localjwt.jwtsigningkey is required when local jwt is enabled")
		}
		switch conf.Auth.Server.LocalJwt.JwtSigningMethod {
		case "HS256", "HS384", "HS512":
		default:
			return fmt.Errorf("auth.server.localjwt.jwtsigningmethod is invalid: %q", conf.Auth.Server.LocalJwt.JwtSigningMethod)
		}
	}

	if conf.Metrics.Enabled {
		if err := validateListenAddress("metrics.listenaddress", conf.Metrics.ListenAddress); err != nil {
			return err
		}
	}

	return nil
}

func validateListenAddress(name, address string) error {
	if _, err := net.ResolveTCPAddr("tcp", address); err != nil {
		return fmt.Errorf("%s is invalid (%q): %w", name, address, err)
	}
	return nil
}

func validateTLSFiles(kind, certFile, keyFile string) error {
	if certFile == "" || keyFile == "" {
		return fmt.Errorf("%s TLS is enabled but the certificate or key file is not configured", kind)
	}
	if _, err := os.Stat(certFile); err != nil {
		return fmt.Errorf("%s TLS certificate %q is not readable: %w", kind, certFile, err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		return fmt.Errorf("%s TLS key %q is not readable: %w", kind, keyFile, err)
	}
	return nil
}
