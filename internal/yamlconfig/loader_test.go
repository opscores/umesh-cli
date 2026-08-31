package yamlconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadYAML(t *testing.T) {
	yamlContent := `
apiVersion: umesh.network/v1
kind: Node
role: genesis
node:
  dataDir: ./data-validator
  moniker: my-validator
  environment: production
chain:
  chainId: umesh-testnet-1
  denom: uumesh
  minGasPrice: "0.0025"
validator:
  keyName: validator
  stakeAmount: "1000000000000uumesh"
  selfDelegation: "1000000000000uumesh"
telemetry:
  endpoint: http://otel-collector:4317
  serviceName: umesh-validator
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "node-config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	cfg, err := LoadYAML(path)
	if err != nil {
		t.Fatalf("LoadYAML failed: %v", err)
	}
	if cfg.APIVersion != "umesh.network/v1" {
		t.Errorf("APIVersion = %q, want umesh.network/v1", cfg.APIVersion)
	}
	if cfg.Kind != "Node" {
		t.Errorf("Kind = %q, want Node", cfg.Kind)
	}
	if cfg.Role != "genesis" {
		t.Errorf("Role = %q, want genesis", cfg.Role)
	}
	if cfg.Chain.ChainID != "umesh-testnet-1" {
		t.Errorf("ChainID = %q", cfg.Chain.ChainID)
	}
	if cfg.Chain.Denom != "uumesh" {
		t.Errorf("Denom = %q", cfg.Chain.Denom)
	}
	if cfg.Validator == nil {
		t.Fatal("expected validator section for genesis")
	}
	if cfg.Validator.KeyName != "validator" {
		t.Errorf("KeyName = %q", cfg.Validator.KeyName)
	}
	if cfg.Telemetry == nil || cfg.Telemetry.Endpoint != "http://otel-collector:4317" {
		t.Errorf("Telemetry endpoint mismatch")
	}
}

func TestValidateInvalidAPIVersion(t *testing.T) {
	cfg := &YamlNodeConfig{
		APIVersion: "invalid",
		Kind:       "Node",
		Role:       "genesis",
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for invalid apiVersion")
	}
}

func TestValidateInvalidRole(t *testing.T) {
	cfg := &YamlNodeConfig{
		APIVersion: "umesh.network/v1",
		Kind:       "Node",
		Role:       "invalid",
		Chain:      ChainInfo{ChainID: "test", Denom: "uume", MinGasPrice: "0.0025"},
		Node:       NodeInfo{Moniker: "test", Environment: "production"},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for invalid role")
	}
}

func TestValidateMissingChainID(t *testing.T) {
	cfg := &YamlNodeConfig{
		APIVersion: "umesh.network/v1",
		Kind:       "Node",
		Role:       "validator",
		Chain:      ChainInfo{Denom: "uume", MinGasPrice: "0.0025"},
		Node:       NodeInfo{Moniker: "test", Environment: "production"},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for missing chainId")
	}
}

func TestValidateJoinForGenesis(t *testing.T) {
	cfg := &YamlNodeConfig{
		APIVersion: "umesh.network/v1",
		Kind:       "Node",
		Role:       "genesis",
		Chain:      ChainInfo{ChainID: "test", Denom: "uume", MinGasPrice: "0.0025"},
		Node:       NodeInfo{Moniker: "test", Environment: "production"},
		Join:       &JoinInfo{GenesisURL: "http://example.com/genesis.json"},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected error for join section on genesis role")
	}
}

func TestValidateRejectsSecretsInConfig(t *testing.T) {
	yamlContent := `
apiVersion: umesh.network/v1
kind: Node
role: validator
node:
  moniker: test
  environment: production
chain:
  chainId: umesh-1
  denom: uumesh
  minGasPrice: "0.0025"
validator:
  keyName: validator
  stakeAmount: "1000000uumesh"
  selfDelegation: "1000000uumesh"
  keyringPassword: "supersecret"
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "node-config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadYAML(path)
	if err == nil {
		t.Error("expected error for secret field in config")
	}
	if err != nil && !strings.Contains(err.Error(), "must not contain secrets") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateJoinerMissingSource(t *testing.T) {
	for _, role := range []string{"validator", "sentry", "rpc"} {
		cfg := &YamlNodeConfig{
			APIVersion: "umesh.network/v1",
			Kind:       "Node",
			Role:       role,
			Chain:      ChainInfo{ChainID: "test", Denom: "uume", MinGasPrice: "0.0025"},
			Node:       NodeInfo{Moniker: "test", Environment: "production"},
		}
		err := Validate(cfg)
		if err == nil {
			t.Errorf("role=%s: expected error for missing join source", role)
		} else if !strings.Contains(err.Error(), "must join an existing network") {
			t.Errorf("role=%s: unexpected error: %v", role, err)
		}
	}
}

func TestValidateJoinerValidSource(t *testing.T) {
	for _, tc := range []struct {
		role string
		join JoinInfo
	}{
		{"validator", JoinInfo{GenesisURL: "https://example.com/genesis.json"}},
		{"validator", JoinInfo{SentryRPC: "http://10.0.0.5:26657"}},
		{"validator", JoinInfo{ValidatorRPC: "http://10.0.0.10:26657"}},
		{"sentry", JoinInfo{SentryRPC: "http://10.0.0.5:26657"}},
		{"rpc", JoinInfo{SentryRPC: "http://10.0.0.5:26657"}},
	} {
		cfg := &YamlNodeConfig{
			APIVersion: "umesh.network/v1",
			Kind:       "Node",
			Role:       tc.role,
			Chain:      ChainInfo{ChainID: "test", Denom: "uume", MinGasPrice: "0.0025"},
			Node:       NodeInfo{Moniker: "test", Environment: "production"},
			Join:       &tc.join,
		}
		if err := Validate(cfg); err != nil {
			t.Errorf("role=%s, join=%+v: unexpected error: %v", tc.role, tc.join, err)
		}
	}
}

func TestValidateJoinerInvalidURL(t *testing.T) {
	cfg := &YamlNodeConfig{
		APIVersion: "umesh.network/v1",
		Kind:       "Node",
		Role:       "validator",
		Chain:      ChainInfo{ChainID: "test", Denom: "uume", MinGasPrice: "0.0025"},
		Node:       NodeInfo{Moniker: "test", Environment: "production"},
		Join:       &JoinInfo{GenesisURL: "not-a-url"},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid URL")
	} else if !strings.Contains(err.Error(), "invalid join source") {
		t.Errorf("unexpected error: %v", err)
	}
}
