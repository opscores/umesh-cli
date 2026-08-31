package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeconfig"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/rpcclient"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show node status",
		Long: `Query the node RPC and print status information.

  umeshctl node status sync       # sync status (height, catching up)
  umeshctl node info              # node info (moniker, network, version) [alias: status node]
  umeshctl node status peers      # connected peers count
  umeshctl node status validator  # validator info (voting power)
  umeshctl node status docker     # docker healthcheck status`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newStatusSyncCmd())
	cmd.AddCommand(newStatusNodeCmd()) // also registered as "info" alias
	cmd.AddCommand(newStatusPeersCmd())
	cmd.AddCommand(newStatusValidatorCmd())
	cmd.AddCommand(newStatusDockerHealthCmd())
	return cmd
}

func newStatusSyncCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Show sync status",
		Long:  "Query the node RPC /status endpoint and print sync information.",
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
				"moniker":           st.NodeInfo.Moniker,
				"network":           st.NodeInfo.Network,
				"block_height":      st.SyncInfo.LatestBlockHeight,
				"block_time":        st.SyncInfo.LatestBlockTime,
				"catching_up":       st.SyncInfo.CatchingUp,
				"voting_power":      st.ValidatorInfo.VotingPower,
			}, func() {
				uio.Print("Moniker:           %s", st.NodeInfo.Moniker)
				uio.Print("Network:           %s", st.NodeInfo.Network)
				uio.Print("Block Height:      %s", st.SyncInfo.LatestBlockHeight)
				uio.Print("Block Time:        %s", st.SyncInfo.LatestBlockTime)
				uio.Print("Catching Up:       %v", st.SyncInfo.CatchingUp)
				uio.Print("Voting Power:      %s", st.ValidatorInfo.VotingPower)
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newStatusNodeCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:     "node",
		Aliases: []string{"info"},
		Short:   "Show node information",
		Long:    "Query the node RPC and display node info (moniker, network, version, listen address).",
		Args:    cobra.NoArgs,
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
				"moniker":     st.NodeInfo.Moniker,
				"network":     st.NodeInfo.Network,
				"node_id":     st.NodeInfo.ID,
				"version":     st.NodeInfo.Version,
				"listen_addr": st.NodeInfo.ListenAddr,
			}, func() {
				uio.Print("Moniker:       %s", st.NodeInfo.Moniker)
				uio.Print("Network:       %s", st.NodeInfo.Network)
				uio.Print("Node ID:       %s", st.NodeInfo.ID)
				uio.Print("Version:       %s", st.NodeInfo.Version)
				uio.Print("Listen Addr:   %s", st.NodeInfo.ListenAddr)
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newStatusPeersCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "peers",
		Short: "Show connected peers",
		Long:  "Query the node RPC and display the number of connected peers.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Connected peers from RPC — best-effort: if RPC is unreachable,
			// still report configured peers from config (mirrors node peers list).
			client := rpcclient.New(global.RPCURL)
			ni, rpcErr := client.NetInfo()
			var connectedPeers int
			if rpcErr == nil {
				connectedPeers = ni.NPeers
			}

			// Configured peers from config
			nc, err := nodeconfig.Load(nodeinit.ConfigDir())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			seeds := nc.Config.GetString("p2p.seeds", "")
			persistent := nc.Config.GetString("p2p.persistent_peers", "")
			unconditional := nc.Config.GetString("p2p.unconditional_peer_ids", "")
			private := nc.Config.GetString("p2p.private_peer_ids", "")

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			if rpcErr != nil {
				uio.LogWarning("Could not query connected peers: %v", rpcErr)
			}
			return uio.Emit(format, map[string]any{
				"connected_peers":        connectedPeers,
				"seeds":                  seeds,
				"persistent_peers":       persistent,
				"unconditional_peer_ids": unconditional,
				"private_peer_ids":       private,
			}, func() {
				uio.Print("Connected Peers: %d", connectedPeers)
				if seeds != "" {
					uio.Print("Seeds:            %s", seeds)
				}
				if persistent != "" {
					uio.Print("Persistent:       %s", persistent)
				}
				if unconditional != "" {
					uio.Print("Unconditional IDs: %s", unconditional)
				}
				if private != "" {
					uio.Print("Private IDs:      %s", private)
				}
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newStatusValidatorCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "validator",
		Short: "Show validator info",
		Long:  "Query the node RPC and display validator information.",
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
				"address":           st.ValidatorInfo.Address,
				"voting_power":      st.ValidatorInfo.VotingPower,
				"proposer_priority": st.ValidatorInfo.ProposerPriority,
			}, func() {
				uio.Print("Validator Address: %s", st.ValidatorInfo.Address)
				uio.Print("Voting Power:     %s", st.ValidatorInfo.VotingPower)
				uio.Print("Proposer Priority: %d", st.ValidatorInfo.ProposerPriority)
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newStatusDockerHealthCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "docker",
		Short: "Show docker healthcheck status",
		Long:  "Check the docker healthcheck status of the node container.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))
			status, err := docker.Inspect("{{.State.Health.Status}}", global.Container)
			if err != nil {
				return fmt.Errorf("failed to check health: %w", err)
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			return uio.Emit(format, map[string]any{
				"docker_health": status,
			}, func() {
				uio.Print("Docker Health: %s", status)
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}
