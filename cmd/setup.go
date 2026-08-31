package cmd

import (
	"github.com/spf13/cobra"
)

// newSetupCmd creates the parent command for all pre-launch operations.
func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Pre-launch setup (plan, tune, keys)",
		Long: `Setup commands run BEFORE launching the node.

Use these to apply tuning profiles and create keys.

  umeshctl setup validate --config config.yaml            # validate node config
  umeshctl setup tune --role validator                    # apply tuning profile
  umeshctl setup keys add validator                       # create validator key`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(
		newTuneCmd(),
		newKeysAddCmd(),
		newValidateConfigCmd(),
	)
	return cmd
}

// newNodeCmd creates the parent command for all runtime operations.
func newNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Runtime operations (status, health, config, peers, logs, prune, snapshot, statesync, upgrade, keys)",
		Long: `Node commands run AFTER the node is launched.

Use these to check status, monitor health, manage config and peers,
view logs, prune data, manage snapshots, configure state sync,
prepare upgrades, and view keys.

  umeshctl node status sync               # sync status
  umeshctl node health                    # health check
  umeshctl node config get <path>         # read config value
  umeshctl node peers list                # list peers
  umeshctl node logs                      # view logs
  umeshctl node prune                     # prune old data
  umeshctl node snapshot create           # create snapshot
  umeshctl node statesync enable          # enable state sync
  umeshctl node upgrade info              # check version
  umeshctl node keys list                 # list keys`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(
		newStatusCmd(),
		newHealthCmd(),
		newConfigCmd(),
		newPeersCmd(),
		newLogsCmd(),
		newPruneCmd(),
		newSnapshotCmd(),
		newStatesyncCmd(),
		newUpgradeCmd(),
		newKeysRuntimeCmd(),
	)
	return cmd
}

// newOpsCmd creates the parent command for maintenance operations.
func newOpsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ops",
		Short: "Maintenance operations (backup, doctor, verify, restore)",
		Long: `Operations commands for node maintenance.

Use these for backups, diagnostics, configuration verification,
and restoring from backup.

  umeshctl ops backup --output ./backups        # backup keys
  umeshctl ops restore --from ./backups/20260810 --role validator
  umeshctl ops doctor                           # diagnostics
  umeshctl ops verify --role validator          # verify config`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(
		newBackupCmd(),
		newRestoreCmd(),
		newDoctorCmd(),
		newVerifyCmd(),
	)
	return cmd
}

// newVerifyCmd creates the verify command (ops phase).
func newVerifyCmd() *cobra.Command {
	var role string
	var crossRole bool
	var sentryRPC string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify node configuration and compliance",
		Long:  "Perform role compliance and connectivity checks against the node RPC.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if crossRole {
				return crossRoleVerify(sentryRPC)
			}
			return roleVerify(role)
		},
	}
	cmd.Flags().StringVar(&role, "role", "validator", "Node role (validator/sentry/rpc)")
	cmd.Flags().BoolVar(&crossRole, "cross-role", false, "Cross-role consistency check")
	cmd.Flags().StringVar(&sentryRPC, "sentry-rpc", "", "Sentry RPC endpoint for cross-role check (overrides SENTRY_RPC env)")
	return cmd
}
