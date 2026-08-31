package cmd

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/nodeinfo"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/secret"
	"github.com/opscores/umesh-cli/internal/uio"
	"github.com/opscores/umesh-cli/internal/yamlconfig"
)

func newInitCmd() *cobra.Command {
	var configFile string
	var force bool
	var keepKeys bool
	var autoPassword bool
	var dryRun bool
	var keyringPasswordFile string
	var keyringPasswordStdin bool
	var keyringPasswordExec string
	var chainID, moniker, denom, minGasPrice, environment, pruning string
	var genesisURL, sentryRPC, validatorRPC, seeds, persistentPeers, externalAddress string
	var addrbookURL, addrbookSHA256, genesisSHA256 string
	var rpcUpstream, restUpstream, p2pUpstream string
	var usePrivate bool
	var publicIP, externalPort string

	cmd := &cobra.Command{
		Use:   "init <role>",
		Short: "Initialize a node for a role",
		Long: `Initialize a node for the given role.

Runs on the host data directory (default by role: data-validator, data-sentry, data-rpc).
Pass --data-dir to override; --home selects the path used inside the container.

CONFIGURATION:
  --config <node-config.yaml>      Typed YAML configuration file (required).
  --auto-password                  Generate a random keyring password and save to disk.
  --keyring-password-file <f>      Read keyring password from a file.
  --keyring-password-stdin         Read keyring password from stdin.
  --keyring-password-exec <cmd>    Execute command and read keyring password from stdout.

Roles:
  genesis    create a NEW network (Block 0) with gentx
  validator  join an EXISTING network as a post-genesis validator
             (requires a genesis source)
  sentry     join as a public peer-facing relay for the validator
  rpc        join as a public RPC node

Validator, sentry, and rpc nodes must join an existing network: at least one
of --genesis-url, --sentry-rpc, --validator-rpc (or --rpc-upstream for rpc)
is required. Flags override config file values.

For full production genesis with docker compose generation, use:
  umeshctl genesis plan --config <node-config.yaml>

Idempotent: if the node is already initialized (genesis.json exists), init
prints a warning and exits successfully. Pass --force to reinitialize
regardless (overwrites existing state).`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("role is required: genesis, validator, sentry, rpc")
			}
			if len(args) > 1 {
				return fmt.Errorf("only one role allowed, got %d", len(args))
			}
			// Validate role value
			if err := validateRole(args[0]); err != nil {
				return err
			}
			return nil
		},
		ValidArgs: validRoles,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeinfo.SetCLIVersion(Version)
			nodeinit.ForceReinit = force
			nodeinit.KeepKeys = keepKeys
			if keepKeys && !force {
				return fmt.Errorf("--keep-keys requires --force (it preserves the node identity keys only during a --force reinit)")
			}
			if force {
				ok, err := uio.Confirm(fmt.Sprintf("Re-initializing role %q with --force will overwrite existing node state. Proceed?", args[0]), force)
				if err != nil {
					return err
				}
				if !ok {
					uio.LogInfo("Aborted.")
					return nil
				}
			}

			role := args[0]
			normalized, err := normalizeRole(role)
			if err != nil {
				return err
			}

			if configFile == "" {
				return fmt.Errorf("--config is required (see 'umeshctl init --help')")
			}

			cfg, err := yamlconfig.LoadYAML(configFile)
			if err != nil {
				return err
			}
			if cfg.Role != normalized {
				return fmt.Errorf("config role is %q but role arg is %q; please align them", cfg.Role, normalized)
			}
			applyFlagOverridesToConfig(cmd, cfg)

			host := ""
			if cmd.Flags().Changed("data-dir") {
				host = global.DataDir
			} else if cfg.Node.DataDir != "" {
				host = cfg.Node.DataDir
			} else {
				host = defaultHome(normalized)
			}
			nodeinit.SetHome(host)
			wasInitialized := nodeinit.GenesisExists()

			keyringPass, err := resolveKeyringPassword(cmd, autoPassword)
			if err != nil {
				return err
			}

			var flowErr error
			if dryRun {
				uio.LogInfo("DRY RUN: would initialize %s node with config %s", normalized, configFile)
				uio.LogInfo("DRY RUN: data dir would be %s", host)
				uio.LogInfo("DRY RUN: keyring password would be saved to %s/keyring.pass", nodeinit.KeyringConfigDir())
				if wasInitialized {
					uio.LogWarning("DRY RUN: node already initialized (genesis.json exists); --force would reinitialize")
				}
				return nil
			}
			switch normalized {
			case "genesis", "validator":
				flowErr = runInit(initNodeWithRole(normalized, cfg, keyringPass))
			case "sentry":
				flowErr = runInit(nodeinit.RunSentry(nodeinit.ToSentryParams(cfg)))
			case "rpc":
				flowErr = runInit(nodeinit.RunRPC(nodeinit.ToRPCParams(cfg)))
			}
			if flowErr != nil {
				return flowErr
			}
			if !wasInitialized || force {
				if err := writeOtel(normalized, cfg); err != nil {
					return err
				}
			}
			printInitSummary(normalized, host, cfg)
			if keyringPass != "" {
				passwordFile, err := saveKeyringPassword(host, keyringPass)
				if err != nil {
					uio.LogWarning("Failed to save keyring password: %v", err)
				} else {
					uio.LogInfo("Keyring password saved to %s", passwordFile)
					uio.LogWarning("This file is outside the data dir; back it up separately from node data backups")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "", "Typed YAML configuration file (required)")
	cmd.Flags().BoolVar(&force, "force", false, "Force reinitialization even if genesis.json exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate config and show what would be done without executing")
	// keepKeys (requires --force): preserves node_key.json + priv_validator_key.json,
	// useful for a sentry re-init without changing its NodeID.
	cmd.Flags().BoolVar(&keepKeys, "keep-keys", false, "Preserve node identity keys (requires --force). For genesis plan, use 'genesis plan --keep-keys' to preserve validator consensus keys instead.")

	cmd.Flags().StringVarP(&keyringPasswordFile, "keyring-password-file", "p", "", "Read keyring password from file (alias: -p)")
	cmd.Flags().BoolVar(&keyringPasswordStdin, "keyring-password-stdin", false, "Read keyring password from stdin")
	cmd.Flags().StringVar(&keyringPasswordExec, "keyring-password-exec", "", "Execute command and read keyring password from stdout")

	cmd.Flags().StringVar(&chainID, "chain-id", "", "Chain ID (overrides config)")
	cmd.Flags().StringVar(&moniker, "moniker", "", "Node moniker (overrides config)")
	cmd.Flags().StringVar(&denom, "denom", "", "Stake denom (overrides config)")
	cmd.Flags().StringVar(&minGasPrice, "min-gas-price", "", "Minimum gas price (overrides config)")
	cmd.Flags().StringVar(&environment, "environment", "", "Network environment: mainnet/testnet/dev (overrides config)")
	cmd.Flags().StringVar(&pruning, "pruning", "", "Pruning strategy (custom, everything, default, nothing) overrides node.pruning in config")

	cmd.Flags().StringVar(&genesisURL, "genesis-url", "", "Genesis document URL for joining a network (overrides config)")
	cmd.Flags().StringVar(&sentryRPC, "sentry-rpc", "", "Sentry RPC endpoint (overrides config)")
	cmd.Flags().StringVar(&validatorRPC, "validator-rpc", "", "Validator RPC endpoint (overrides config)")
	cmd.Flags().StringVar(&seeds, "seeds", "", "Comma-separated seed peers (overrides config)")
	cmd.Flags().StringVar(&persistentPeers, "persistent-peers", "", "Comma-separated persistent peers (overrides config)")
	cmd.Flags().StringVar(&externalAddress, "external-address", "", "External p2p address ip:port (overrides config)")
	cmd.Flags().StringVar(&addrbookURL, "addrbook-url", "", "Addrbook URL (overrides config)")
	cmd.Flags().StringVar(&addrbookSHA256, "addrbook-sha256", "", "Addrbook SHA-256 checksum (overrides config)")
	cmd.Flags().StringVar(&genesisSHA256, "genesis-sha256", "", "Genesis SHA-256 checksum (overrides config)")

	cmd.Flags().StringVar(&rpcUpstream, "rpc-upstream", "", "Upstream RPC for a public rpc node (overrides config)")
	cmd.Flags().StringVar(&restUpstream, "rest-upstream", "", "Upstream REST for a public rpc node (overrides config)")
	cmd.Flags().StringVar(&p2pUpstream, "p2p-upstream", "", "Upstream p2p address for a public rpc node (overrides config)")
	cmd.Flags().BoolVar(&usePrivate, "use-private", false, "(sentry) Register the validator as a private peer (overrides config)")
	cmd.Flags().StringVar(&publicIP, "public-ip", "", "Public IP for sentry (overrides config)")
	cmd.Flags().StringVar(&externalPort, "external-port", "", "External p2p port (overrides config)")

	cmd.Flags().BoolVar(&autoPassword, "auto-password", false, "Generate a random keyring password and save it to disk")
	_ = cmd.RegisterFlagCompletionFunc("config", completeYAMLFiles())
	_ = cmd.RegisterFlagCompletionFunc("pruning", completePrune())
	return cmd
}

func resolveKeyringPassword(cmd *cobra.Command, autoGenerate bool) (string, error) {
	f := cmd.Flags()
	if f.Changed("keyring-password-file") || f.Changed("keyring-password-stdin") || f.Changed("keyring-password-exec") {
		src := secret.Source{}
		if v, _ := f.GetString("keyring-password-file"); v != "" {
			src.File = v
		}
		if b, _ := f.GetBool("keyring-password-stdin"); b {
			src.Stdin = true
		}
		if v, _ := f.GetString("keyring-password-exec"); v != "" {
			src.Exec = v
		}
		return secret.Resolve(src)
	}
	if autoGenerate {
		return generateKeyringPassword()
	}
	if pass, err := uio.ReadSecret("Enter keyring password: "); err == nil {
		return pass, nil
	}
	return "", nil
}

func generateKeyringPassword() (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}

func saveKeyringPassword(dataDir string, password string) (string, error) {
	// Store keyring password in XDG config dir (~/.config/umesh/keyring.pass)
	// instead of inside the data directory to avoid accidental inclusion in backups.
	configDir := nodeinit.KeyringConfigDir()
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return "", fmt.Errorf("create keyring config dir: %w", err)
	}
	filePath := filepath.Join(configDir, "keyring.pass")
	if err := os.WriteFile(filePath, []byte(password), 0o600); err != nil {
		return "", fmt.Errorf("write keyring password file: %w", err)
	}
	return filePath, nil
}

func initNodeWithRole(role string, cfg *yamlconfig.YamlNodeConfig, keyringPass string) error {
	switch role {
	case "genesis":
		return nodeinit.RunGenesis(nodeinit.ToGenesisParams(cfg, keyringPass))
	case "validator":
		return nodeinit.RunValidator(nodeinit.ToValidatorParams(cfg, keyringPass))
	default:
		return fmt.Errorf("unexpected role %q for password-protected init", role)
	}
}

func applyFlagOverridesToConfig(cmd *cobra.Command, cfg *yamlconfig.YamlNodeConfig) {
	f := cmd.Flags()

	// String flag mappings: flag name -> pointer to destination
	stringFlags := []struct {
		flag string
		dst  *string
	}{
		{"chain-id", &cfg.Chain.ChainID},
		{"moniker", &cfg.Node.Moniker},
		{"denom", &cfg.Chain.Denom},
		{"min-gas-price", &cfg.Chain.MinGasPrice},
		{"environment", &cfg.Node.Environment},
		{"pruning", &cfg.Node.Pruning},
	}

	for _, m := range stringFlags {
		if f.Changed(m.flag) {
			v, _ := f.GetString(m.flag)
			*m.dst = v
		}
	}

	// Join config
	if cfg.Join == nil {
		cfg.Join = &yamlconfig.JoinInfo{}
	}
	joinStringFlags := []struct {
		flag string
		dst  *string
	}{
		{"genesis-url", &cfg.Join.GenesisURL},
		{"sentry-rpc", &cfg.Join.SentryRPC},
		{"validator-rpc", &cfg.Join.ValidatorRPC},
		{"genesis-sha256", &cfg.Join.GenesisSHA256},
	}
	for _, m := range joinStringFlags {
		if f.Changed(m.flag) {
			v, _ := f.GetString(m.flag)
			*m.dst = v
		}
	}
	// Legacy --rpc-upstream flag maps to join.sentryRpc for RPC role
	if f.Changed("rpc-upstream") {
		v, _ := f.GetString("rpc-upstream")
		cfg.Join.SentryRPC = v
	}

	// Network config
	if cfg.Network == nil {
		cfg.Network = &yamlconfig.NetworkInfo{}
	}
	networkStringFlags := []struct {
		flag string
		dst  *string
	}{
		{"seeds", &cfg.Network.Seeds},
		{"persistent-peers", &cfg.Network.PersistentPeers},
		{"external-address", &cfg.Network.ExternalAddress},
		{"public-ip", &cfg.Network.PublicIP},
		{"external-port", &cfg.Network.ExternalPort},
	}
	for _, m := range networkStringFlags {
		if f.Changed(m.flag) {
			v, _ := f.GetString(m.flag)
			*m.dst = v
		}
	}

	// Bool flags
	boolFlags := []struct {
		flag string
		dst  *bool
	}{
		{"use-private", &cfg.Network.UsePrivate},
	}
	for _, m := range boolFlags {
		if f.Changed(m.flag) {
			v, _ := f.GetBool(m.flag)
			*m.dst = v
		}
	}
}

func normalizeRole(role string) (string, error) {
	switch role {
	case "genesis", "validator", "sentry", "rpc":
		return role, nil
	}
	return "", fmt.Errorf("unknown role %q: must be one of genesis, validator, sentry, rpc", role)
}

func validateRole(role string) error {
	switch role {
	case "genesis", "validator", "sentry", "rpc":
		return nil
	}
	return fmt.Errorf("unknown role %q: must be one of genesis, validator, sentry, rpc", role)
}

func defaultHome(role string) string {
	switch role {
	case "sentry":
		return "./data-sentry"
	case "rpc":
		return "./data-rpc"
	default:
		return "./data-validator"
	}
}

func composeProfile(role string) string {
	switch role {
	case "sentry":
		return "sentry"
	case "rpc":
		return "rpc"
	default:
		return "validator"
	}
}

func printInitSummary(role, host string, cfg *yamlconfig.YamlNodeConfig) {
	uio.Print("Node initialized: role=%s", role)
	uio.Print("  data dir:  %s", host)
	if cfg != nil {
		if cfg.Node.Moniker != "" {
			uio.Print("  moniker:   %s", cfg.Node.Moniker)
		}
		if cfg.Chain.ChainID != "" {
			uio.Print("  chain id:  %s", cfg.Chain.ChainID)
		}
	}
	if info, err := nodeinfo.Read(host); err == nil && info.NodeID != "" {
		uio.Print("  node id:   %s", info.NodeID)
	}
	if nodeinit.OtelEnabled() {
		uio.Print("  telemetry: OTLP enabled (see data-<role>/config/otel.yaml)")
	} else {
		uio.Print("  telemetry: off (set telemetry.endpoint in config and re-run init)")
	}
	uio.Print("Next step — start the node:")
	uio.Print("  docker compose --profile %s up -d", composeProfile(role))
}

func writeOtel(role string, cfg *yamlconfig.YamlNodeConfig) error {
	var endpoint, envName, serviceName string
	if cfg != nil {
		if cfg.Telemetry != nil {
			endpoint = cfg.Telemetry.Endpoint
			serviceName = cfg.Telemetry.ServiceName
		}
		envName = cfg.Node.Environment
	}
	if serviceName == "" {
		switch role {
		case "genesis":
			serviceName = "umesh-validator"
		default:
			serviceName = "umesh-" + role
		}
	}
	if err := nodeinit.WriteOtelConfig(serviceName, endpoint, envName); err != nil {
		return err
	}
	return nil
}

func runInit(err error) error {
	if errors.Is(err, nodeinit.ErrAlreadyInitialized) {
		uio.LogWarning("%s", err.Error())
		uio.LogInfo("To reinitialize, remove the node home dir and re-run:")
		uio.LogInfo("  umeshctl init <role> --force --config <file>")
		return nil
	}
	return err
}

func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
