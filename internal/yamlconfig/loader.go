package yamlconfig

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var secretFieldPatterns = []string{
	"keyringPassword",
	"keyringpassword",
	"password",
	"secret",
	"mnemonic",
	"privValidator",
	"priv_validator",
}

func LoadYAML(path string) (*YamlNodeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	var rawMap map[string]any
	if err := yaml.Unmarshal(data, &rawMap); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	if err := validateNoSecretsFromRaw(rawMap); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}

	var cfg YamlNodeConfig
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &cfg, nil
}

func validateNoSecretsFromRaw(v any) error {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			kl := strings.ToLower(k)
			for _, pattern := range secretFieldPatterns {
				if strings.Contains(kl, pattern) {
					return fmt.Errorf("field %q must not contain secrets; use --keyring-password-file/stdin/exec instead", k)
				}
			}
			if err := validateNoSecretsFromRaw(val); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range t {
			if err := validateNoSecretsFromRaw(item); err != nil {
				return err
			}
		}
	}
	return nil
}


