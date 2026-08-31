package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/nodeconfig"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newRPCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rpc",
		Short: "Public RPC node operations",
		Long: `RPC-specific operations for public nodes.
These commands require the node to be initialized as an RPC node (role=rpc).`,
		Annotations: map[string]string{"role-guard": "rpc"},
	}
	cmd.AddCommand(newRPCSetUpstreamCmd())
	cmd.AddCommand(newRPCConfigureCorsCmd())
	return cmd
}

func newRPCSetUpstreamCmd() *cobra.Command {
	var rpcUpstream, restUpstream, p2pUpstream string
	cmd := &cobra.Command{
		Use:   "set-upstream",
		Short: "Configure upstream RPC/REST/P2P endpoints",
		Long: `Set the upstream endpoints for the public RPC node.
The RPC node proxies requests to these upstream endpoints.

At least one upstream must be specified:
  --rpc-upstream    RPC endpoint URL (e.g. http://sentry:26657)
  --rest-upstream   REST endpoint URL (e.g. http://sentry:1317)
  --p2p-upstream    P2P node ID@host:port for persistent peering (e.g. abc123@10.0.0.5:26656)

The P2P upstream is added to p2p.persistent_peers.
RPC/REST upstreams are stored in app.toml [upstream] section for reference by proxy tooling.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rpcUpstream == "" && restUpstream == "" && p2pUpstream == "" {
				return fmt.Errorf("at least one of --rpc-upstream, --rest-upstream, or --p2p-upstream is required")
			}

			nc, err := nodeconfig.Load(nodeinit.ConfigDir())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			updated := 0

			if p2pUpstream != "" {
				cur := nc.Config.GetString("p2p.persistent_peers", "")
				merged := nodeconfig.MergePeerID(cur, p2pUpstream)
				if err := nc.Set(nc.Config, "p2p.persistent_peers", merged); err != nil {
					return fmt.Errorf("set p2p.persistent_peers: %w", err)
				}
				uio.LogSuccess("Updated p2p.persistent_peers: %s", merged)
				updated++
			}

			if rpcUpstream != "" {
				if err := nc.Set(nc.App, "upstream.rpc_url", rpcUpstream); err != nil {
					return fmt.Errorf("set upstream.rpc_url: %w", err)
				}
				uio.LogSuccess("Set upstream.rpc_url: %s", rpcUpstream)
				updated++
			}

			if restUpstream != "" {
				if err := nc.Set(nc.App, "upstream.rest_url", restUpstream); err != nil {
					return fmt.Errorf("set upstream.rest_url: %w", err)
				}
				uio.LogSuccess("Set upstream.rest_url: %s", restUpstream)
				updated++
			}

			uio.LogSuccess("Updated %d of 3 upstream endpoints", updated)

			return nil
		},
	}

	cmd.Flags().StringVar(&rpcUpstream, "rpc-upstream", "", "Upstream RPC endpoint URL")
	cmd.Flags().StringVar(&restUpstream, "rest-upstream", "", "Upstream REST endpoint URL")
	cmd.Flags().StringVar(&p2pUpstream, "p2p-upstream", "", "Upstream P2P node ID@host:port")
	return cmd
}

func newRPCConfigureCorsCmd() *cobra.Command {
	var origins []string
	var disable bool
	cmd := &cobra.Command{
		Use:   "configure-cors",
		Short: "Configure RPC CORS allowed origins",
		Long: `Manage the rpc.cors_allowed_origins setting in config.toml.

Default (no flags): enables CORS with ["*"] (allow all).
  --origins <list>  Comma-separated list of allowed origins (e.g. https://app.example.com,https://app2.example.com)
  --disable         Set cors_allowed_origins to empty (disable CORS)

The change is written to config.toml and takes effect on next node restart.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			nc, err := nodeconfig.Load(nodeinit.ConfigDir())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			var value any
			if disable {
				value = []string{}
			} else if len(origins) > 0 {
				value = origins
			} else {
				value = []string{"*"}
			}

			if err := nc.Set(nc.Config, "rpc.cors_allowed_origins", value); err != nil {
				return fmt.Errorf("set rpc.cors_allowed_origins: %w", err)
			}

			if disable {
				uio.LogSuccess("CORS disabled (rpc.cors_allowed_origins = [])")
			} else if len(origins) > 0 {
				uio.LogSuccess("CORS configured: %v", origins)
			} else {
				uio.LogSuccess("CORS enabled for all origins (*)")
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&origins, "origins", nil, "Comma-separated CORS allowed origins")
	cmd.Flags().BoolVar(&disable, "disable", false, "Disable CORS (set empty list)")
	return cmd
}
