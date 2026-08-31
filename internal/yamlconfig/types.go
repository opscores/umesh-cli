package yamlconfig

import (
	"fmt"
	"net/url"
	"strings"
)

type YamlNodeConfig struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Role       string         `yaml:"role"`
	Node       NodeInfo       `yaml:"node"`
	Chain      ChainInfo      `yaml:"chain"`
	Validator  *ValidatorInfo `yaml:"validator,omitempty"`
	Join       *JoinInfo      `yaml:"join,omitempty"`
	Network    *NetworkInfo   `yaml:"network,omitempty"`
	Telemetry  *TelemetryInfo `yaml:"telemetry,omitempty"`
}

type NodeInfo struct {
	DataDir     string `yaml:"dataDir"`
	Moniker     string `yaml:"moniker"`
	Environment string `yaml:"environment"`
	Pruning     string `yaml:"pruning"`
}

type ChainInfo struct {
	ChainID     string `yaml:"chainId"`
	Denom       string `yaml:"denom"`
	MinGasPrice string `yaml:"minGasPrice"`
	DenomURI    string `yaml:"denomUri"`
}

type ValidatorInfo struct {
	KeyName           string         `yaml:"keyName"`
	StakeAmount       string         `yaml:"stakeAmount"`
	SelfDelegation    string         `yaml:"selfDelegation"`
	ExternalAddress   string         `yaml:"externalAddress"`
	Commission        CommissionInfo `yaml:"commission"`
	MinSelfDelegation string         `yaml:"minSelfDelegation"`
}

type CommissionInfo struct {
	Rate          string `yaml:"rate"`
	MaxRate       string `yaml:"maxRate"`
	MaxChangeRate string `yaml:"maxChangeRate"`
}

type JoinInfo struct {
	GenesisURL    string `yaml:"genesisUrl"`
	GenesisSHA256 string `yaml:"genesisSha256"`
	SentryRPC     string `yaml:"sentryRpc"`
	ValidatorRPC  string `yaml:"validatorRpc"`
}

type NetworkInfo struct {
	Seeds           string `yaml:"seeds"`
	PersistentPeers string `yaml:"persistentPeers"`
	ExternalAddress string `yaml:"externalAddress"`
	UsePrivate      bool   `yaml:"usePrivate"`
	PublicIP        string `yaml:"publicIp"`
	ExternalPort    string `yaml:"externalPort"`
}

type TelemetryInfo struct {
	Endpoint    string `yaml:"endpoint"`
	ServiceName string `yaml:"serviceName"`
}

func Validate(cfg *YamlNodeConfig) error {
	if cfg.APIVersion != "umesh.network/v1" {
		return fmt.Errorf("apiVersion must be 'umesh.network/v1', got %q", cfg.APIVersion)
	}
	if cfg.Kind != "Node" {
		return fmt.Errorf("kind must be 'Node', got %q", cfg.Kind)
	}
	if cfg.Role == "" {
		return fmt.Errorf("role is required")
	}
	switch cfg.Role {
	case "genesis", "validator", "sentry", "rpc":
	default:
		return fmt.Errorf("role must be one of: genesis, validator, sentry, rpc; got %q", cfg.Role)
	}
	if cfg.Chain.ChainID == "" && cfg.Join == nil {
		return fmt.Errorf("chain.chainId is required when join section is not specified (auto-extracted from genesis when join is set)")
	}
	if cfg.Chain.Denom == "" && cfg.Join == nil {
		return fmt.Errorf("chain.denom is required when join section is not specified")
	}
	if cfg.Chain.MinGasPrice == "" {
		return fmt.Errorf("chain.minGasPrice is required")
	}
	if cfg.Node.Moniker == "" {
		return fmt.Errorf("node.moniker is required")
	}
	if cfg.Node.Environment == "" {
		return fmt.Errorf("node.environment is required")
	}
	if cfg.Node.Pruning != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Node.Pruning)) {
		case "custom", "everything", "default", "nothing":
		default:
			return fmt.Errorf("node.pruning must be one of: custom, everything, default, nothing; got %q", cfg.Node.Pruning)
		}
	}
	if cfg.Validator != nil && cfg.Role != "genesis" && cfg.Role != "validator" {
		return fmt.Errorf("validator section is only valid for genesis and validator roles")
	}
	if cfg.Join != nil && cfg.Role == "genesis" {
		return fmt.Errorf("join section is not valid for genesis role")
	}

	// Joiner roles (validator, sentry, rpc) must join an existing network by
	// configuring at least one genesis source. The genesis role creates a new
	// network and never joins, so this only applies to joiners.
	if cfg.Role != "genesis" {
		if err := requireJoinSource(cfg.Role, cfg.Join); err != nil {
			return err
		}
	}

	return nil
}

// requireJoinSource enforces that a joiner role (validator, sentry, rpc) has at
// least one configured genesis source, and that any configured source URL is a
// valid http(s) URL.
func requireJoinSource(role string, join *JoinInfo) error {
	if join == nil {
		return fmt.Errorf("role=%s must join an existing network: set at least one of join.genesisUrl, join.sentryRpc, join.validatorRpc (or join.sentryRpc for rpc)", role)
	}

	// rpc can join via sentryRpc (preferred) or genesisUrl/validatorRpc as fallback
	// (priority: sentryRpc → genesisUrl → validatorRpc).
	if role == "rpc" {
		if join.SentryRPC == "" && join.GenesisURL == "" && join.ValidatorRPC == "" {
			return fmt.Errorf("role=rpc must join an existing network: set at least one of join.sentryRpc or join.genesisUrl (join.validatorRpc also accepted)")
		}
		for _, s := range []string{join.SentryRPC, join.GenesisURL, join.ValidatorRPC} {
			if s != "" {
				if err := requireValidURL(s); err != nil {
					return err
				}
			}
		}
		return nil
	}

	sources := []string{join.GenesisURL, join.SentryRPC, join.ValidatorRPC}
	any := false
	for _, s := range sources {
		if s != "" {
			any = true
			if err := requireValidURL(s); err != nil {
				return err
			}
		}
	}
	if !any {
		return fmt.Errorf("role=%s must join an existing network: set at least one of join.genesisUrl, join.sentryRpc, join.validatorRpc", role)
	}
	return nil
}

// requireValidURL fails if s is not a valid http(s) URL with a scheme and host.
func requireValidURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid join source %q: must be a valid URL with scheme and host (e.g. https://host/genesis.json)", s)
	}
	return nil
}
