package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/nodeconfig"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/role"
	"github.com/opscores/umesh-cli/internal/rpcclient"
	"github.com/opscores/umesh-cli/internal/uio"
)

// newConfigCmd creates the parent command for config operations (runtime).
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage node configuration",
		Long: `Read and write configuration parameters.

  umeshctl node config get <path>         # read a value
  umeshctl node config set <path> <val>   # write a value
  umeshctl node config diff               # compare with best practices (auto-detects role)`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigDiffCmd())
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Get a configuration value",
		Long: `Read a parameter from config.toml or app.toml by dot-separated path.

Examples:
  umeshctl node config get consensus.timeout_commit
  umeshctl node config get p2p.max_num_inbound_peers
  umeshctl node config get rpc.laddr`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			nc, err := nodeconfig.Load(nodeinit.ConfigDir())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Try config.toml first, then app.toml
			val, ok := nc.Config.Get(path)
			if !ok {
				val, ok = nc.App.Get(path)
			}
			if !ok {
				return fmt.Errorf("path %q not found in config.toml or app.toml", path)
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			return uio.Emit(format, map[string]any{path: val}, func() {
				uio.Print("%v", val)
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newConfigSetCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "set <path> <value>",
		Short: "Set a configuration value",
		Long: `Write a parameter to config.toml or app.toml by dot-separated path.

Examples:
  umeshctl node config set p2p.max_num_inbound_peers 60
  umeshctl node config set rpc.laddr "tcp://0.0.0.0:26657"
  umeshctl node config set consensus.timeout_commit 5s`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, err := uio.Confirm(fmt.Sprintf("Write %s = %s to config?", args[0], args[1]), yes)
			if err != nil {
				return err
			}
			if !ok {
				uio.LogInfo("Aborted.")
				return nil
			}
			path := args[0]
			valueStr := args[1]

			nc, err := nodeconfig.Load(nodeinit.ConfigDir())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Parse value (try int, then float, then string)
			var value any
			if i, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
				value = i
			} else if f, err := strconv.ParseFloat(valueStr, 64); err == nil {
				value = f
			} else if valueStr == "true" {
				value = true
			} else if valueStr == "false" {
				value = false
			} else {
				value = valueStr
			}

			// Try to set in config.toml first, then app.toml
			if err := nc.Set(nc.Config, path, value); err != nil {
				if err := nc.Set(nc.App, path, value); err != nil {
					return fmt.Errorf("set %q: path not found in config.toml or app.toml", path)
				}
				uio.LogSuccess("Set app.toml %s = %v", path, value)
			} else {
				uio.LogSuccess("Set config.toml %s = %v", path, value)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func newConfigDiffCmd() *cobra.Command {
	var roleOverride string
	var output string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare config with best practices",
		Long: `Show differences between current config and recommended values for the role.

Example:
  umeshctl node config diff                    # auto-detect role
  umeshctl node config diff --role sentry      # override role`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedRole, err := role.Resolve(global.DataDir, roleOverride)
			if err != nil {
				return err
			}

			nc, err := nodeconfig.Load(nodeinit.ConfigDir())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			var items []diffItem
			// Basic checks
			switch resolvedRole {
			case "validator", "genesis":
				items = append(items, checkConfig(nc, "p2p.pex", false))
				items = append(items, checkConfig(nc, "statesync.enable", false))
				items = append(items, checkConfig(nc, "api.enable", false))
			case "sentry":
				items = append(items, checkConfig(nc, "p2p.pex", true))
				items = append(items, checkConfig(nc, "statesync.enable", true))
				items = append(items, checkConfig(nc, "api.enable", true))
				items = append(items, checkConfig(nc, "api.address", "tcp://0.0.0.0:1317"))
				items = append(items, checkConfig(nc, "grpc.address", "0.0.0.0:9090"))
			case "rpc":
				items = append(items, checkConfig(nc, "p2p.pex", true))
				items = append(items, checkConfig(nc, "statesync.enable", true))
				items = append(items, checkConfig(nc, "api.enable", true))
				items = append(items, checkConfig(nc, "api.address", "tcp://0.0.0.0:1317"))
				items = append(items, checkConfig(nc, "grpc.address", "0.0.0.0:9090"))
			}

			// Container-aware address checks: every node type runs in its own
			// Docker container on its own VPS, so ports that are published on
			// the host must bind 0.0.0.0 in-container (a loopback bind is not
			// reachable through docker port publishing), and the node must
			// advertise a reachable p2p.external_address instead of its bridge IP.
			items = append(items, checkConfig(nc, "rpc.laddr", "tcp://0.0.0.0:26657"))
			items = append(items, checkConfig(nc, "p2p.laddr", "tcp://0.0.0.0:26656"))
			items = append(items, checkExternalAddress(nc))

			warn := 0
			for _, it := range items {
				if it.Status == "warn" {
					warn++
				}
			}
			report := map[string]any{
				"role":   resolvedRole,
				"checks": items,
				"summary": map[string]int{
					"ok":   len(items) - warn,
					"warn": warn,
				},
			}
			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			return uio.Emit(format, report, func() {
				uio.LogStep("Comparing config with best practices for role=%s", resolvedRole)
				for _, it := range items {
					switch it.Status {
					case "ok":
						uio.LogSuccess("  %s = %s (OK)", it.Path, it.Current)
					case "warn":
						if it.Current == "" && it.Expected != "" {
							uio.LogWarning("  %s: MISSING (expected %s)", it.Path, it.Expected)
						} else {
							uio.LogWarning("  %s = %s (expected %s)", it.Path, it.Current, it.Expected)
						}
					}
				}
				if warn > 0 {
					uio.LogWarning("Found %d config issue(s) — see above", warn)
				} else {
					uio.LogSuccess("Config matches best practices")
				}
			})
		},
	}
	cmd.Flags().StringVar(&roleOverride, "role", "", "Node role override (auto-detected if empty)")
	_ = cmd.RegisterFlagCompletionFunc("role", completeRoles())
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func roleVerify(role string) error {
	client := rpcclient.New(global.RPCURL)
	uio.LogStep("Verifying %s role", role)

	if err := client.Health(); err != nil {
		uio.Fatal(fmt.Errorf("node is not responding: %w", err))
	}
	uio.LogSuccess("Health check passed")

	st, err := client.Status()
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}
	uio.LogSuccess("Network: %s (moniker %s)", st.NodeInfo.Network, st.NodeInfo.Moniker)

	ni, err := client.NetInfo()
	if err != nil {
		return fmt.Errorf("net info: %w", err)
	}
	if ni.NPeers < 1 && role != "validator" {
		uio.Fatal(&verifyErr{msg: "no peers connected"})
	}
	uio.LogSuccess("Peer connectivity OK (%d peers)", ni.NPeers)
	uio.LogSuccess("All %s role checks passed", role)
	return nil
}

type verifyErr struct{ msg string }

func (e *verifyErr) Error() string { return e.msg }

func crossRoleVerify(sentryRPCFlag string) error {
	sentryURL := sentryRPCFlag
	if sentryURL == "" {
		sentryURL = envOr("SENTRY_RPC", "")
	}
	if sentryURL == "" {
		sentryURL = envOr("SENTRY_RPC_FALLBACK", "")
	}
	if sentryURL == "" {
		uio.LogWarning("--sentry-rpc not set and SENTRY_RPC/SENTRY_RPC_FALLBACK env vars not set; using --rpc-url as the sentry check target")
		sentryURL = global.RPCURL
	}

	validatorClient := rpcclient.New(global.RPCURL)
	sentryClient := rpcclient.New(sentryURL)

	vs, err := validatorClient.Status()
	if err != nil {
		return fmt.Errorf("validator status: %w", err)
	}
	ss, err := sentryClient.Status()
	if err != nil {
		return fmt.Errorf("sentry status: %w", err)
	}

	if vs.NodeInfo.Network != ss.NodeInfo.Network {
		uio.Fatal(&verifyErr{msg: fmt.Sprintf("chain-id mismatch: validator=%s sentry=%s", vs.NodeInfo.Network, ss.NodeInfo.Network)})
	}
	uio.LogSuccess("[1/3] chain-id matches: %s", vs.NodeInfo.Network)

	uio.LogStep("[2/3] heights")
	uio.Print("  Validator: %s", vs.SyncInfo.LatestBlockHeight)
	uio.Print("  Sentry:    %s", ss.SyncInfo.LatestBlockHeight)

	vn, err := validatorClient.NetInfo()
	sn, err2 := sentryClient.NetInfo()
	if err == nil {
		uio.LogSuccess("[3/3] validator peers: %d", vn.NPeers)
	}
	if err2 == nil {
		uio.LogSuccess("[3/3] sentry peers: %d", sn.NPeers)
	}
	if err == nil && vn.NPeers < 1 {
		uio.LogWarning("validator has no peers")
	}
	if err2 == nil && sn.NPeers < 1 {
		uio.LogWarning("sentry has no peers")
	}
	uio.LogSuccess("Cross-role verification complete")
	return nil
}

// diffItem is a single config comparison result.
type diffItem struct {
	Path     string `json:"path" yaml:"path"`
	Current  string `json:"current,omitempty" yaml:"current,omitempty"`
	Expected string `json:"expected,omitempty" yaml:"expected,omitempty"`
	Status   string `json:"status" yaml:"status"` // ok, warn
}

func checkConfig(nc *nodeconfig.NodeConfig, path string, expected any) diffItem {
	val, ok := nc.Config.Get(path)
	if !ok {
		val, ok = nc.App.Get(path)
	}
	if !ok {
		return diffItem{Path: path, Expected: fmt.Sprintf("%v", expected), Status: "warn"}
	}
	if fmt.Sprintf("%v", val) == fmt.Sprintf("%v", expected) {
		return diffItem{Path: path, Current: fmt.Sprintf("%v", val), Expected: fmt.Sprintf("%v", expected), Status: "ok"}
	}
	return diffItem{Path: path, Current: fmt.Sprintf("%v", val), Expected: fmt.Sprintf("%v", expected), Status: "warn"}
}

// checkExternalAddress warns when p2p.external_address is unset or loopback.
// Inside a container CometBFT introspects its listener and advertises the
// bridge IP, which peers on other VPS hosts cannot dial.
func checkExternalAddress(nc *nodeconfig.NodeConfig) diffItem {
	addr := nc.Config.GetString("p2p.external_address", "")
	switch {
	case addr == "":
		return diffItem{Path: "p2p.external_address", Status: "warn"}
	case strings.HasPrefix(addr, "tcp://"):
		return diffItem{Path: "p2p.external_address", Current: addr, Status: "warn"}
	case strings.HasPrefix(addr, "127.") || strings.HasPrefix(addr, "localhost"):
		return diffItem{Path: "p2p.external_address", Current: addr, Status: "warn"}
	default:
		return diffItem{Path: "p2p.external_address", Current: addr, Status: "ok"}
	}
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
