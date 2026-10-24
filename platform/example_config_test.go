package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// examplePlatformConfig is the subset of the platform configuration that
// points at files shipped (or not) alongside an example.
type examplePlatformConfig struct {
	HTTP struct {
		Server struct {
			TLSEnabled      bool
			TLSCertFileName string
			TLSKeyFileName  string
		}
	}
	Vault struct {
		Enabled bool
		Token   struct {
			Enabled   bool
			TokenPath string
		}
		Cert struct {
			Enabled  bool
			CertFile string
			KeyFile  string
		}
	}
}

// TestExampleConfigsReferenceExistingFiles enforces the runnable-example
// contract: a feature that needs a file may only be enabled when the file
// ships with the example. Files referenced by disabled features are ignored.
func TestExampleConfigsReferenceExistingFiles(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate the repository root")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile))
	examplesRoot := filepath.Join(repoRoot, "example")

	var configs []string
	err := filepath.WalkDir(examplesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "config.yml" {
			configs = append(configs, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking examples: %v", err)
	}
	if len(configs) == 0 {
		t.Fatal("no example config.yml files found")
	}

	for _, configPath := range configs {
		configPath := configPath
		t.Run(strings.TrimPrefix(configPath, examplesRoot+string(filepath.Separator)), func(t *testing.T) {
			loader := viper.New()
			loader.SetConfigFile(configPath)
			loader.SetConfigType("yml")
			if err := loader.ReadInConfig(); err != nil {
				t.Fatalf("reading %s: %v", configPath, err)
			}

			var cfg examplePlatformConfig
			if err := loader.UnmarshalKey("platform", &cfg); err != nil {
				t.Fatalf("decoding %s: %v", configPath, err)
			}

			configDir := filepath.Dir(configPath)
			requireFile := func(feature, field, name string) {
				if name == "" {
					t.Errorf("%s is enabled but %s is empty", feature, field)
					return
				}
				resolved := name
				if !filepath.IsAbs(resolved) {
					resolved = filepath.Join(configDir, resolved)
				}
				info, err := os.Stat(resolved)
				if err != nil {
					t.Errorf("%s is enabled but %s points at missing file %q", feature, field, name)
					return
				}
				if info.IsDir() {
					t.Errorf("%s is enabled but %s points at a directory %q", feature, field, name)
				}
			}

			if cfg.HTTP.Server.TLSEnabled {
				requireFile("http server TLS", "tlscertfilename", cfg.HTTP.Server.TLSCertFileName)
				requireFile("http server TLS", "tlskeyfilename", cfg.HTTP.Server.TLSKeyFileName)
			}
			if cfg.Vault.Enabled {
				if cfg.Vault.Cert.Enabled {
					requireFile("vault cert auth", "CertFile", cfg.Vault.Cert.CertFile)
					requireFile("vault cert auth", "KeyFile", cfg.Vault.Cert.KeyFile)
				}
				if cfg.Vault.Token.Enabled && cfg.Vault.Token.TokenPath != "" {
					requireFile("vault token file", "TokenPath", cfg.Vault.Token.TokenPath)
				}
			}
		})
	}
}
