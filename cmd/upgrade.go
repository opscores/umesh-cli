package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/rpcclient"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Manage software upgrades",
		Long: `View current version and prepare for software upgrades.

  umeshctl node upgrade info                          # current version + available upgrades
  umeshctl node upgrade prepare --version v0.2.0      # download binary for upgrade`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newUpgradeInfoCmd())
	cmd.AddCommand(newUpgradePrepareCmd())
	return cmd
}

func newUpgradeInfoCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show current version and upgrade info",
		Long:  "Query the node for its current version and check for available upgrades.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := rpcclient.New(global.RPCURL)

			st, err := client.Status()
			if err != nil {
				return fmt.Errorf("failed to connect to node: %w", err)
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			return uio.Emit(format, map[string]any{
				"version": st.NodeInfo.Version,
				"network": st.NodeInfo.Network,
			}, func() {
				uio.Print("Current Version:  %s", st.NodeInfo.Version)
				uio.Print("Network:          %s", st.NodeInfo.Network)
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newUpgradePrepareCmd() *cobra.Command {
	var version string
	var yes bool

	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare for a software upgrade (NOT YET IMPLEMENTED)",
		Long: `⚠️ NOT YET IMPLEMENTED — this command will return an error.

Download the binary for a future software upgrade.
This prepares the binary that will be used when the upgrade block height is reached.
Requires cosmovisor or similar upgrade management.

  umeshctl node upgrade prepare --version v0.2.0`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if version == "" {
				return fmt.Errorf("--version is required")
			}

			uio.LogWarning("upgrade prepare is not implemented yet: automatic binary download and checksum verification are not wired up")
			return fmt.Errorf("upgrade prepare is not implemented yet")
		},
	}

	cmd.Flags().StringVar(&version, "version", "", "Target version (e.g. v0.2.0)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}
