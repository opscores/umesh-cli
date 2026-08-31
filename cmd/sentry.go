package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeconfig"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newSentryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "sentry",
		Short:       "Manage Sentry node operations",
		Long:        `Sentry node management: connect to validator and manage peers.`,
		Annotations: map[string]string{"role-guard": "sentry"},
	}
	cmd.AddCommand(newSentryConnectCmd())
	cmd.AddCommand(newSentryUpdateCmd())
	return cmd
}

func newSentryConnectCmd() *cobra.Command {
	var sentryRPC, validatorRPC string
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Connect Sentry node to Validator",
		Long: `Resolve node IDs for sentry and validator via RPC and display
the connection pair. The operator configures the actual peer relationship
via umeshctl init --role sentry or sentry update.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if sentryRPC == "" || validatorRPC == "" {
				return fmt.Errorf("--sentry-rpc and --validator-rpc are required")
			}
			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))
			sentryID, err := rpcNodeID(docker, sentryRPC)
			if err != nil {
				return fmt.Errorf("resolve sentry node id: %w", err)
			}
			valID, err := rpcNodeID(docker, validatorRPC)
			if err != nil {
				return fmt.Errorf("resolve validator node id: %w", err)
			}
			uio.LogSuccess("validator %s <-> sentry %s", valID, sentryID)
			uio.LogInfo("Next step — configure peering on validator:")
			uio.LogInfo("  umeshctl sentry update --peer-id %s", sentryID)
			return nil
		},
	}
	cmd.Flags().StringVar(&sentryRPC, "sentry-rpc", "", "Sentry RPC endpoint")
	cmd.Flags().StringVar(&validatorRPC, "validator-rpc", "", "Validator RPC endpoint")
	return cmd
}

func newSentryUpdateCmd() *cobra.Command {
	var peerID string
	var yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update validator peer list with sentry node ID",
		Long: `Add a sentry node ID to the validator's unconditional/private peer lists.
Used to establish private peering between validator and sentry.

Runs inside the container via docker exec. The container must be running.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if peerID == "" {
				return fmt.Errorf("--peer-id is required")
			}

			ok, err := uio.Confirm(fmt.Sprintf("Add sentry peer %s to validator's unconditional/private peer lists?", peerID), yes)
			if err != nil {
				return err
			}
			if !ok {
				uio.LogInfo("Aborted.")
				return nil
			}

			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))

			home := global.Home
			configFile := home + "/config/config.toml"

			for _, key := range []string{"p2p.unconditional_peer_ids", "p2p.private_peer_ids"} {
				// Read current value using dasel inside container
				cur, err := docker.ExecOutput("dasel", "-f", configFile, "-r", "toml", key)
				if err != nil {
					cur = ""
				}
				cur = strings.TrimSpace(cur)

				// Merge peer ID
				merged := nodeconfig.MergePeerID(cur, peerID)
				if merged == cur {
					continue
				}

				// Write back using dasel inside container
				if _, err := docker.Exec(nil, "dasel", "put", "-f", configFile, "-t", "string", "-v", merged, key); err != nil {
					return fmt.Errorf("update %s: %w", key, err)
				}
				uio.LogSuccess("updated %s: %s", key, merged)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&peerID, "peer-id", "", "Sentry node ID to add (required)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	_ = cmd.MarkFlagRequired("peer-id")
	return cmd
}

// rpcNodeID fetches the node id from an RPC /status endpoint.
func rpcNodeID(docker *dkrcmd.Docker, rpc string) (string, error) {
	out, err := docker.ExecOutput("sh", "-c", fmt.Sprintf("curl -s %s/status", rpc))
	if err != nil {
		return "", err
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return "", err
	}
	result, _ := m["result"].(map[string]any)
	ni, _ := result["node_info"].(map[string]any)
	id, _ := ni["id"].(string)
	return strings.TrimSpace(id), nil
}
