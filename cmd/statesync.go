package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/nodeconfig"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newStatesyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "statesync",
		Short: "Manage state sync configuration",
		Long: `Enable, disable, or show state sync configuration.

State sync allows a node to quickly catch up by downloading a snapshot
and verifying it against trusted block height and hash.

  umeshctl node statesync enable --trust-height 12345 --trust-hash <hash>
  umeshctl node statesync disable
  umeshctl node statesync show`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newStatesyncEnableCmd())
	cmd.AddCommand(newStatesyncDisableCmd())
	cmd.AddCommand(newStatesyncShowCmd())
	return cmd
}

func newStatesyncShowCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current state sync configuration",
		Long:  "Display the current state sync settings from config.toml.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			nc, err := nodeconfig.Load(nodeinit.ConfigDir())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}

			enabled := nc.Config.GetBool("statesync.enable", false)
			trustHeight := nc.Config.GetInt64("statesync.trust_height", 0)
			trustHash := nc.Config.GetString("statesync.trust_hash", "")
			rpcServers := nc.Config.GetString("statesync.rpc_servers", "")

			return uio.Emit(format, map[string]any{
				"enabled":       enabled,
				"trust_height":  trustHeight,
				"trust_hash":    trustHash,
				"rpc_servers":   rpcServers,
			}, func() {
				if enabled {
					uio.LogSuccess("State sync: ENABLED")
				} else {
					uio.Print("State sync: disabled")
				}
				if trustHeight > 0 {
					uio.Print("Trust Height:  %d", trustHeight)
				}
				if trustHash != "" {
					uio.Print("Trust Hash:    %s", trustHash)
				}
				if rpcServers != "" {
					uio.Print("RPC Servers:   %s", rpcServers)
				}
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newStatesyncEnableCmd() *cobra.Command {
	var trustHeight int64
	var trustHash, rpcServers string
	var yes bool

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable state sync",
		Long: `Enable state sync with trusted block height and hash.

You need to provide:
  - trust-height: the height of a trusted block (must be > 0)
  - trust-hash: the hash of the trusted block
  - rpc-servers: optional, RPC servers to fetch data from

Example:
  umeshctl node statesync enable --trust-height 100000 --trust-hash ABC123...
  umeshctl node statesync enable --trust-height 100000 --trust-hash ABC123... --rpc-servers http://node1:26657,http://node2:26657`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("trust-height") {
				return fmt.Errorf("--trust-height is required")
			}
			if trustHeight <= 0 {
				return fmt.Errorf("--trust-height must be > 0")
			}
			if trustHash == "" {
				return fmt.Errorf("--trust-hash is required")
			}

			ok, err := uio.Confirm("Enable state sync with provided trust parameters?", yes)
			if err != nil {
				return err
			}
			if !ok {
				uio.LogInfo("Aborted.")
				return nil
			}

			nc, err := nodeconfig.Load(nodeinit.ConfigDir())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Enable state sync
			if err := nc.Set(nc.Config, "statesync.enable", true); err != nil {
				return fmt.Errorf("set statesync.enable: %w", err)
			}
			if err := nc.Set(nc.Config, "statesync.trust_height", trustHeight); err != nil {
				return fmt.Errorf("set statesync.trust_height: %w", err)
			}
			if err := nc.Set(nc.Config, "statesync.trust_hash", trustHash); err != nil {
				return fmt.Errorf("set statesync.trust_hash: %w", err)
			}

			// Set RPC servers if provided
			if rpcServers != "" {
				if err := nc.Set(nc.Config, "statesync.rpc_servers", rpcServers); err != nil {
					return fmt.Errorf("set statesync.rpc_servers: %w", err)
				}
			}

			uio.LogSuccess("State sync enabled (trust_height=%d, trust_hash=%s)", trustHeight, trustHash)
			return nil
		},
	}

	cmd.Flags().Int64Var(&trustHeight, "trust-height", 0, "Trusted block height (required, must be > 0)")
	cmd.Flags().StringVar(&trustHash, "trust-hash", "", "Trusted block hash (required)")
	cmd.Flags().StringVar(&rpcServers, "rpc-servers", "", "Comma-separated list of RPC servers")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func newStatesyncDisableCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable state sync",
		Long:  "Disable state sync in the node configuration.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, err := uio.Confirm("Disable state sync?", yes)
			if err != nil {
				return err
			}
			if !ok {
				uio.LogInfo("Aborted.")
				return nil
			}
			nc, err := nodeconfig.Load(nodeinit.ConfigDir())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if err := nc.Set(nc.Config, "statesync.enable", false); err != nil {
				return fmt.Errorf("set statesync.enable: %w", err)
			}

			uio.LogSuccess("State sync disabled")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}
