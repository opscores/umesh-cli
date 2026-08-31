package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/rpcclient"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newHealthCmd() *cobra.Command {
	var waitSync bool
	var timeout time.Duration
	var output string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Quick health check",
		Long: `Check if the node is healthy and responding.

By default, performs a single health check.
Use --wait-sync to block until the node finishes syncing.

  umeshctl node health                    # quick check
  umeshctl node health --wait-sync        # wait until caught up
  umeshctl node health --wait-sync --timeout 5m
  umeshctl node health --output json      # machine-readable`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := rpcclient.New(global.RPCURL)
			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}

			if err := client.Health(); err != nil {
				return fmt.Errorf("node is not responding: %w", err)
			}

			// --- --wait-sync: block until the node is fully synchronized ---
			if waitSync {
				var syncErr error
				if format != uio.FormatTable {
					syncErr = waitForSyncQuiet(client, timeout)
				} else {
					syncErr = waitForSync(client, timeout)
				}
				if syncErr != nil {
					return syncErr
				}
				st, err := client.Status()
				if err != nil {
					return fmt.Errorf("failed to get status: %w", err)
				}
				if st.SyncInfo.LatestBlockHeight == "0" || st.SyncInfo.LatestBlockHeight == "" {
					return fmt.Errorf("node reported height 0 after sync — not fully caught up")
				}
				return uio.Emit(format, healthResult{
					Healthy:    true,
					CatchingUp: false,
					Height:     st.SyncInfo.LatestBlockHeight,
				}, func() {
					uio.LogSuccess("Node is fully synced (height: %s)", st.SyncInfo.LatestBlockHeight)
				})
			}

			// --- single health check (no --wait-sync) ---
			st, err := client.Status()
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}

			res := healthResult{
				Healthy:    true,
				CatchingUp: st.SyncInfo.CatchingUp,
				Height:     st.SyncInfo.LatestBlockHeight,
				BlockTime:  st.SyncInfo.LatestBlockTime,
				Moniker:    st.NodeInfo.Moniker,
				Network:    st.NodeInfo.Network,
			}
			if format != uio.FormatTable {
				return uio.Emit(format, res, func() {})
			}
			uio.LogSuccess("Node is responding")
			if st.SyncInfo.CatchingUp {
				uio.LogWarning("Node is still catching up (height: %s)", st.SyncInfo.LatestBlockHeight)
			} else {
				uio.LogSuccess("Node is fully synced (height: %s)", st.SyncInfo.LatestBlockHeight)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&waitSync, "wait-sync", false, "Wait until node is fully synced")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Timeout for --wait-sync")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())

	return cmd
}

// healthResult is the structured output of node health.
type healthResult struct {
	Healthy    bool   `json:"healthy" yaml:"healthy"`
	CatchingUp bool   `json:"catching_up" yaml:"catching_up"`
	Height     string `json:"height" yaml:"height"`
	BlockTime  string `json:"block_time,omitempty" yaml:"block_time,omitempty"`
	Moniker    string `json:"moniker,omitempty" yaml:"moniker,omitempty"`
	Network    string `json:"network,omitempty" yaml:"network,omitempty"`
}

func waitForSync(client *rpcclient.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	uio.LogInfo("Waiting for node to sync (timeout: %s)...", timeout)

	for {
		select {
		case <-ticker.C:
			st, err := client.Status()
			if err != nil {
				uio.LogWarning("Failed to query status: %v", err)
				continue
			}
			if !st.SyncInfo.CatchingUp {
				uio.LogSuccess("Node is fully synced (height: %s)", st.SyncInfo.LatestBlockHeight)
				return nil
			}
			uio.LogInfo("Syncing... height: %s", st.SyncInfo.LatestBlockHeight)
		default:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for sync")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// waitForSyncQuiet is like waitForSync but suppresses stdout progress output
// so it doesn't pollute JSON/YAML output. Only fatal/warning logs are emitted.
func waitForSyncQuiet(client *rpcclient.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			st, err := client.Status()
			if err != nil {
				uio.LogWarning("Failed to query status: %v", err)
				continue
			}
			if !st.SyncInfo.CatchingUp {
				return nil
			}
		default:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for sync")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}
