package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/nodeconfig"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/rpcclient"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newPeersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "peers",
		Short: "Manage P2P peers",
		Long: `Manage persistent peers and peer IDs.

  umeshctl node peers list                           # list all peers
  umeshctl node peers add <node-id>@<ip>:26656       # add persistent peer
  umeshctl node peers remove <node-id>               # remove peer
  umeshctl node peers clear                          # clear all persistent peers`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newPeersListCmd())
	cmd.AddCommand(newPeersAddCmd())
	cmd.AddCommand(newPeersRemoveCmd())
	cmd.AddCommand(newPeersClearCmd())
	return cmd
}

func newPeersListCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured peers",
		Long:  "Display seeds, persistent peers, and peer IDs from config and connected peers from RPC.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Connected peers from RPC (best-effort: if RPC is unreachable,
			// still report configured peers from config).
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
				// tableFn is only invoked for FormatTable, so stdout stays clean
				// for --output json/yaml.
				if rpcErr == nil {
					uio.Print("Connected Peers: %d", connectedPeers)
				}
				if seeds != "" {
					uio.Print("Seeds:              %s", seeds)
				}
				if persistent != "" {
					uio.Print("Persistent:         %s", persistent)
				}
				if unconditional != "" {
					uio.Print("Unconditional IDs:  %s", unconditional)
				}
				if private != "" {
					uio.Print("Private IDs:       %s", private)
				}
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newPeersAddCmd() *cobra.Command {
	var persistent, unconditional, yes bool
	cmd := &cobra.Command{
		Use:   "add <node-id>@<ip>:<port>",
		Short: "Add a persistent peer",
		Long: `Add a peer to the persistent_peers list.

The peer argument must have the form "<node-id>@<ip>:<port>", e.g.
abc123def456...@1.2.3.4:26656.

Examples:
  umeshctl node peers add abc123...@1.2.3.4:26656
  umeshctl node peers add --persistent abc123...@1.2.3.4:26656
  umeshctl node peers add --unconditional abc123...

Flags:
  --persistent      add to p2p.persistent_peers (default)
  --unconditional    add to p2p.unconditional_peer_ids
  -y, --yes          skip confirmation prompt`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			peer := args[0]
			if !isValidPeerArg(peer) {
				return fmt.Errorf("invalid peer %q: expected <node-id>@<ip>:<port> (e.g. a1b2...@1.2.3.4:26656)", peer)
			}

			ok, err := uio.Confirm(fmt.Sprintf("Add peer %s to %s?", peer, func() string {
				if unconditional {
					return "unconditional_peer_ids"
				}
				return "persistent_peers"
			}()), yes)
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

			if unconditional {
				// Add to unconditional_peer_ids
				key := "p2p.unconditional_peer_ids"
				cur := nc.Config.GetString(key, "")
				merged := nodeconfig.MergePeerID(cur, peer)
				if err := nc.Set(nc.Config, key, merged); err != nil {
					return fmt.Errorf("set %s: %w", key, err)
				}
				uio.LogSuccess("Added %s to unconditional_peer_ids", peer)
			} else {
				// Add to persistent_peers - use exact match to avoid partial matches
				key := "p2p.persistent_peers"
				cur := nc.Config.GetString(key, "")
				if cur != "" {
					peers := strings.Split(cur, ",")
					found := false
					for _, p := range peers {
						if p == peer {
							found = true
							break
						}
					}
					if !found {
						cur += "," + peer
					}
				} else {
					cur = peer
				}
				if err := nc.Set(nc.Config, key, cur); err != nil {
					return fmt.Errorf("set %s: %w", key, err)
				}
				uio.LogSuccess("Added %s to persistent_peers", peer)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&persistent, "persistent", true, "Add as persistent peer")
	cmd.Flags().BoolVar(&unconditional, "unconditional", false, "Add as unconditional peer ID")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func newPeersRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <node-id>",
		Short: "Remove a peer",
		Long: `Remove a peer from persistent_peers and peer ID lists by node ID.
The <node-id> is the CometBFT node ID (hex, e.g. abc123def456...).

Example:
  umeshctl node peers remove abc123def456...`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeConfiguredPeerIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Accept both a bare node ID and "<node-id>@<host>:<port>"; extract
			// the node ID so validation/removal works in either form.
			nodeID := args[0]
			if at := strings.LastIndex(nodeID, "@"); at > 0 {
				nodeID = nodeID[:at]
			}
			if !isValidNodeID(nodeID) {
				return fmt.Errorf("invalid node ID %q: expected 40- or 128-character hex node ID (e.g. abc123def456...)", nodeID)
			}

			ok, err := uio.Confirm(fmt.Sprintf("Remove peer %s from all peer lists?", nodeID), yes)
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

			// Remove from persistent_peers
			persistentKey := "p2p.persistent_peers"
			cur := nc.Config.GetString(persistentKey, "")
			if cur != "" {
				// Split by comma, filter out the exact peer match, rejoin
				peers := strings.Split(cur, ",")
				var filtered []string
				for _, p := range peers {
					if p != nodeID+"@"+strings.Split(p, "@")[1] && !strings.HasPrefix(p, nodeID+"@") {
						filtered = append(filtered, p)
					}
				}
				newVal := strings.Join(filtered, ",")
				if err := nc.Set(nc.Config, persistentKey, newVal); err != nil {
					return fmt.Errorf("set %s: %w", persistentKey, err)
				}
			}

			// Remove from unconditional_peer_ids
			uncondKey := "p2p.unconditional_peer_ids"
			cur = nc.Config.GetString(uncondKey, "")
			if cur != "" {
				ids := strings.Split(cur, ",")
				var filtered []string
				for _, id := range ids {
					if id != nodeID {
						filtered = append(filtered, id)
					}
				}
				newVal := strings.Join(filtered, ",")
				if err := nc.Set(nc.Config, uncondKey, newVal); err != nil {
					return fmt.Errorf("set %s: %w", uncondKey, err)
				}
			}

			uio.LogSuccess("Removed %s from peer lists", nodeID)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func newPeersClearCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear all persistent peers",
		Long:  "Remove all persistent_peers from config.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, err := uio.Confirm("Clear ALL persistent peers from config?", yes)
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

			if err := nc.Set(nc.Config, "p2p.persistent_peers", ""); err != nil {
				return fmt.Errorf("clear persistent_peers: %w", err)
			}
			uio.LogSuccess("Cleared all persistent peers")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

// isValidPeerArg reports whether s looks like a CometBFT peer address of the
// form "<node-id>@<host>:<port>" (the node ID is a hex string, host may be IP
// or DNS, port numeric). This is a cheap format check, not a full validation.
func isValidPeerArg(s string) bool {
	at := strings.LastIndex(s, "@")
	if at <= 0 || at == len(s)-1 {
		return false
	}
	hostPort := s[at+1:]
	colon := strings.LastIndex(hostPort, ":")
	if colon <= 0 || colon == len(hostPort)-1 {
		return false
	}
	return isValidNodeID(s[:at])
}

// isValidNodeID reports whether s is a plausible CometBFT node ID: a hex string
// of 40 (ed25519) or 128 characters.
func isValidNodeID(s string) bool {
	if len(s) != 40 && len(s) != 128 {
		return false
	}
	for _, r := range s {
		if !isHexDigit(r) {
			return false
		}
	}
	return true
}

// isHexDigit reports whether r is an ASCII hex digit.
func isHexDigit(r rune) bool {
	switch {
	case '0' <= r && r <= '9':
		return true
	case 'a' <= r && r <= 'f':
		return true
	case 'A' <= r && r <= 'F':
		return true
	default:
		return false
	}
}

// completeConfiguredPeerIDs offers the peer IDs currently configured in
// p2p.persistent_peers / p2p.unconditional_peer_ids / p2p.private_peer_ids as
// completion candidates for `peer remove`.
func completeConfiguredPeerIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	nc, err := nodeconfig.Load(nodeinit.ConfigDir())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var ids []string
	for _, key := range []string{"p2p.persistent_peers", "p2p.unconditional_peer_ids", "p2p.private_peer_ids"} {
		raw := nc.Config.GetString(key, "")
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			id := p
			if i := strings.LastIndex(p, "@"); i >= 0 {
				id = p[:i]
			}
			if isValidNodeID(id) {
				ids = append(ids, id)
			}
		}
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}
