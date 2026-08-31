// Package tune applies production tuning to a node's config.toml and app.toml.
package tune

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/opscores/umesh-cli/internal/nodeconfig"
)

// Role determines which tuning profile to apply.
type Role string

const (
	RoleValidator Role = "validator"
	RoleGenesis   Role = "genesis"
	RoleSentry    Role = "sentry"
	RoleRPC       Role = "rpc"
)

// Options configures the tune.Apply operation.
type Options struct {
	Environment      string
	Moniker          string
	Denom            string
	MinGasPrice      string
	ExternalAddress  string
	ChainID          string
	Pruning          string
	AllowDuplicateIP bool
}

// =============================================================================
// Consensus timeouts
// =============================================================================
const (
	TimeoutCommit             = "3s"
	TimeoutPropose            = "3s"
	TimeoutProposeDelta       = "500ms"
	TimeoutPrevote            = "1s"
	TimeoutPrevoteDelta       = "500ms"
	TimeoutPrecommit          = "1s"
	TimeoutPrecommitDelta     = "500ms"
	CreateEmptyBlocksInterval = "0s"
	P2PHandshakeTimeout       = "20s"
	P2PDialTimeout            = "3s"
	P2PFlushThrottleTimeout   = "100ms"
)

// =============================================================================
// P2P bandwidth rates (bytes/sec)
// =============================================================================
const (
	P2PSendRate = int64(5120000) // 5 MB/s
	P2PRecvRate = int64(5120000) // 5 MB/s
)

// =============================================================================
// Peer limits per role
// =============================================================================
const (
	ValidatorMaxInboundPeers  = int64(5)
	ValidatorMaxOutboundPeers = int64(4)
	SentryMaxInboundPeers     = int64(40)
	SentryMaxOutboundPeers    = int64(20)
	RPCMaxInboundPeers        = int64(60)
	RPCMaxOutboundPeers       = int64(40)
)

// =============================================================================
// Peer security
// =============================================================================
const (
	// Must stay 0: on a single-validator network the node's own signature is in
	// every block, so checking recent commits trips ErrSignatureFoundInPastBlocks
	// on every restart. CometBFT default is 0 too.
	DoubleSignCheckHeight = int64(0) // validator only; 0 for sentry/rpc
)

// =============================================================================
// Mempool configuration
// =============================================================================
const (
	ValidatorMempoolSize     = int64(5000)
	ValidatorMempoolMaxBytes = int64(256 * 1024 * 1024) // 256 MB
	SentryMempoolSize        = int64(10000)
	SentryMempoolMaxBytes    = int64(1024 * 1024 * 1024) // 1 GB
	RPCMempoolSize           = int64(5000)
	RPCMempoolMaxBytes       = int64(1024 * 1024 * 1024) // 1 GB
	ValidatorMaxTxs          = int64(-1)                 // unlimited
	SentryMaxTxs             = int64(10000)
	RPCMaxTxs                = int64(5000)
)

// =============================================================================
// RPC limits (rate limiting for public nodes)
// =============================================================================
const (
	SentryRPCMaxConnections    = int64(2000)
	SentryRPCMaxSubClients     = int64(100)
	SentryRPCMaxSubPerClient   = int64(20)
	RPCRPCMaxConnections       = int64(4000)
	RPCRPCMaxSubClients        = int64(200)
	RPCRPCMaxSubPerClient      = int64(50)
	ValidatorRPCMaxConnections = int64(100) // low for private validator
)

// =============================================================================
// API / gRPC limits
// =============================================================================
const (
	PublicAPIMaxConnections = int64(2000)              // sentry + rpc
	MaxRecvMsgSize          = int64(100 * 1024 * 1024) // 100 MB
	MaxSendMsgSize          = int64(100 * 1024 * 1024) // 100 MB
	MinRetainBlocks         = int64(100)
)

// =============================================================================
// Telemetry
// =============================================================================
const (
	TelemetryRetentionTime = int64(100)
)

// =============================================================================
// WASM configuration
// =============================================================================
const (
	WASMSimulationGasLimit = int64(10_000_000)
	WASMSmartQueryGasLimit = int64(3_000_000)
	WASMMemoryCacheSize    = int64(256)
)

// =============================================================================
// IAVL cache sizes
// =============================================================================
const (
	ValidatorIAVLCache = int64(781250)
	PublicIAVLCache    = int64(1562500) // sentry + rpc
)

// =============================================================================
// Pruning settings
// =============================================================================
const (
	ValidatorPruningKeepRecent = "10000"
	ValidatorPruningInterval   = "1000"
	PublicPruningKeepRecent    = "1000" // sentry + rpc
	PublicPruningInterval      = "100"
)

// =============================================================================
// State sync snapshots
// =============================================================================
const (
	SentrySnapshotInterval = int64(5000)
	SnapshotKeepRecent     = int64(2)
	DisabledSnapshot       = int64(0)
)

// =============================================================================
// Client (client.toml)
// =============================================================================
const (
	KeyringBackend = "file"
)

// Apply writes the production tuning profile for role into configDir.
func Apply(configDir string, role Role, opts ...Options) error {
	nc, err := nodeconfig.Load(configDir)
	if err != nil {
		return err
	}

	opt := Options{
		Environment: "production",
		Moniker:     "validator",
		Denom:       "uumesh",
		MinGasPrice: "0.0025",
	}
	if len(opts) > 0 {
		if opts[0].Environment != "" {
			opt.Environment = opts[0].Environment
		}
		if opts[0].Moniker != "" {
			opt.Moniker = opts[0].Moniker
		}
		if opts[0].Denom != "" {
			opt.Denom = opts[0].Denom
		}
		if opts[0].MinGasPrice != "" {
			opt.MinGasPrice = opts[0].MinGasPrice
		}
		opt.ExternalAddress = opts[0].ExternalAddress
		if opts[0].ChainID != "" {
			opt.ChainID = opts[0].ChainID
		}
		if opts[0].Pruning != "" {
			opt.Pruning = strings.ToLower(strings.TrimSpace(opts[0].Pruning))
		}
		opt.AllowDuplicateIP = opts[0].AllowDuplicateIP
	}

	normalizedRole := role
	if role == RoleGenesis {
		normalizedRole = RoleValidator
	}

	if err := applyConfigTuning(nc, normalizedRole, opt); err != nil {
		return fmt.Errorf("apply config tuning: %w", err)
	}
	if err := applyAppTuning(nc, normalizedRole, opt, configDir); err != nil {
		return fmt.Errorf("apply app tuning: %w", err)
	}
	if err := applyClientTuning(nc, opt); err != nil {
		return fmt.Errorf("apply client tuning: %w", err)
	}

	return nil
}

func applyConfigTuning(nc *nodeconfig.NodeConfig, role Role, opt Options) error {
	configMap := map[string]any{
		"log_format":                             "json",
		"log_level":                              "info",
		"tx_index.indexer":                       "kv",
		"consensus.timeout_commit":               TimeoutCommit,
		"consensus.timeout_propose":              TimeoutPropose,
		"consensus.timeout_propose_delta":        TimeoutProposeDelta,
		"consensus.timeout_prevote":              TimeoutPrevote,
		"consensus.timeout_prevote_delta":        TimeoutPrevoteDelta,
		"consensus.timeout_precommit":            TimeoutPrecommit,
		"consensus.timeout_precommit_delta":      TimeoutPrecommitDelta,
		"consensus.create_empty_blocks":          true,
		"consensus.create_empty_blocks_interval": CreateEmptyBlocksInterval,
		"consensus.skip_timeout_commit":          false,
		"instrumentation.prometheus":             true,
		"instrumentation.prometheus_listen_addr": ":26660",
		"p2p.laddr":                              "tcp://0.0.0.0:26656",
		"p2p.handshake_timeout":                  P2PHandshakeTimeout,
		"p2p.dial_timeout":                       P2PDialTimeout,
		"p2p.flush_throttle_timeout":             P2PFlushThrottleTimeout,
		"p2p.send_rate":                          P2PSendRate,
		"p2p.recv_rate":                          P2PRecvRate,
		"p2p.addr_book_strict":                   false,
		"p2p.allow_duplicate_ip":                 opt.AllowDuplicateIP,
		"consensus.double_sign_check_height":     DoubleSignCheckHeight,
		"rpc.laddr":                              "tcp://0.0.0.0:26657",
		"rpc.cors_allowed_origins":               []string{},
		"mempool.cache_size":                     int64(10000),
	}

	switch role {
	case RoleSentry:
		configMap["moniker"] = opt.Moniker + "-sentry"
		configMap["p2p.pex"] = true
		configMap["p2p.addr_book_strict"] = true
		configMap["consensus.double_sign_check_height"] = int64(0)
		configMap["p2p.max_num_inbound_peers"] = SentryMaxInboundPeers
		configMap["p2p.max_num_outbound_peers"] = SentryMaxOutboundPeers
		configMap["mempool.size"] = SentryMempoolSize
		configMap["mempool.max_txs_bytes"] = SentryMempoolMaxBytes
		configMap["statesync.enable"] = true
		configMap["rpc.max_open_connections"] = SentryRPCMaxConnections
		configMap["rpc.max_subscription_clients"] = SentryRPCMaxSubClients
		configMap["rpc.max_subscriptions_per_client"] = SentryRPCMaxSubPerClient
		configMap["rpc.cors_allowed_origins"] = []string{"*"}
		configMap["data_companion.enabled"] = true
		configMap["data_companion.grpc_address"] = "0.0.0.0:26658"

	case RoleRPC:
		configMap["moniker"] = opt.Moniker + "-rpc"
		configMap["p2p.pex"] = true
		configMap["p2p.addr_book_strict"] = true
		configMap["consensus.double_sign_check_height"] = int64(0)
		configMap["p2p.max_num_inbound_peers"] = RPCMaxInboundPeers
		configMap["p2p.max_num_outbound_peers"] = RPCMaxOutboundPeers
		configMap["mempool.size"] = RPCMempoolSize
		configMap["mempool.max_txs_bytes"] = RPCMempoolMaxBytes
		configMap["statesync.enable"] = true
		configMap["rpc.max_open_connections"] = RPCRPCMaxConnections
		configMap["rpc.max_subscription_clients"] = RPCRPCMaxSubClients
		configMap["rpc.max_subscriptions_per_client"] = RPCRPCMaxSubPerClient
		configMap["rpc.cors_allowed_origins"] = []string{"*"}
		configMap["data_companion.enabled"] = false
		configMap["rpc.max_body_bytes"] = int64(10_000_000)  // 10 MB request body
		configMap["rpc.max_header_bytes"] = int64(1_048_576) // 1 MB headers
		configMap["rpc.timeout_broadcast_tx_commit"] = "10s"

	case RoleValidator:
		configMap["moniker"] = opt.Moniker
		configMap["tx_index.indexer"] = "null"
		configMap["p2p.pex"] = false
		configMap["consensus.double_sign_check_height"] = DoubleSignCheckHeight
		configMap["p2p.max_num_inbound_peers"] = ValidatorMaxInboundPeers
		configMap["p2p.max_num_outbound_peers"] = ValidatorMaxOutboundPeers
		configMap["mempool.size"] = ValidatorMempoolSize
		configMap["mempool.max_txs_bytes"] = ValidatorMempoolMaxBytes
		configMap["statesync.enable"] = false
		configMap["rpc.max_open_connections"] = ValidatorRPCMaxConnections
		configMap["data_companion.enabled"] = false
	}

	// Each node type runs in its own Docker container on its own VPS, so the
	// node must advertise a reachable P2P address. Without p2p.external_address
	// CometBFT introspects the listener and advertises the container's bridge
	// IP (172.x.x.x), which peers on other hosts cannot dial.
	if addr := nodeconfig.NormalizeExternalAddress(opt.ExternalAddress); addr != "" {
		configMap["p2p.external_address"] = addr
	}

	for k, v := range configMap {
		if err := nc.Set(nc.Config, k, v); err != nil {
			return err
		}
	}
	return nil
}

func applyAppTuning(nc *nodeconfig.NodeConfig, role Role, opt Options, configDir string) error {
	appMap := map[string]any{
		"minimum-gas-prices":                  opt.MinGasPrice + opt.Denom,
		"telemetry.enabled":                   true,
		"telemetry.service-name":              "umesh-" + string(role),
		"telemetry.enable-hostname":           true,
		"telemetry.enable-service-label":      true,
		"telemetry.prometheus-retention-time": TelemetryRetentionTime,
		"telemetry.global-labels":             [][]string{{"environment", opt.Environment}, {"role", string(role)}},
		"iavl-disable-fastnode":               false,
		"wasm.simulation_gas_limit":           WASMSimulationGasLimit,
		"wasm.smart_query_gas_limit":          WASMSmartQueryGasLimit,
		"wasm.memory_cache_size":              WASMMemoryCacheSize,
		"wasm.data_dir":                       filepath.Join(filepath.Dir(configDir), "wasm"),
	}

	switch role {
	case RoleSentry, RoleRPC:
		appMap["api.enable"] = true
		appMap["api.address"] = "tcp://0.0.0.0:1317"
		appMap["api.enabled-unsafe-cors"] = false
		appMap["api.swagger"] = true
		appMap["api.max-open-connections"] = PublicAPIMaxConnections
		appMap["grpc.enable"] = true
		appMap["grpc.address"] = "0.0.0.0:9090"
		appMap["grpc-web.enable"] = true
		appMap["grpc.max-recv-msg-size"] = MaxRecvMsgSize
		appMap["grpc.max-send-msg-size"] = MaxSendMsgSize
		appMap["pruning"] = "custom"
		appMap["pruning-keep-recent"] = PublicPruningKeepRecent
		appMap["pruning-interval"] = PublicPruningInterval
		appMap["iavl-cache-size"] = PublicIAVLCache
		appMap["min-retain-blocks"] = MinRetainBlocks

		if role == RoleSentry {
			appMap["state-sync.snapshot-interval"] = SentrySnapshotInterval
			appMap["state-sync.snapshot-keep-recent"] = SnapshotKeepRecent
			appMap["mempool.max-txs"] = SentryMaxTxs
		} else {
			appMap["state-sync.snapshot-interval"] = DisabledSnapshot
			appMap["state-sync.snapshot-keep-recent"] = DisabledSnapshot
			appMap["mempool.max-txs"] = RPCMaxTxs
		}

	case RoleValidator:
		appMap["api.enable"] = false
		appMap["api.address"] = "tcp://127.0.0.1:1317"
		appMap["api.enabled-unsafe-cors"] = false
		appMap["grpc.enable"] = false
		appMap["grpc.address"] = "127.0.0.1:9090"
		appMap["grpc-web.enable"] = false
		appMap["pruning"] = "custom"
		appMap["pruning-keep-recent"] = ValidatorPruningKeepRecent
		appMap["pruning-interval"] = ValidatorPruningInterval
		appMap["state-sync.snapshot-interval"] = DisabledSnapshot
		appMap["state-sync.snapshot-keep-recent"] = DisabledSnapshot
		appMap["mempool.max-txs"] = ValidatorMaxTxs
		appMap["iavl-cache-size"] = ValidatorIAVLCache
		appMap["min-retain-blocks"] = MinRetainBlocks
	}

	// Override pruning if explicitly set via node.pruning (local instance setting).
	// chain.pruning is intentionally not used — pruning is per-node, not per-chain.
	if opt.Pruning != "" {
		appMap["pruning"] = opt.Pruning
		// For custom we keep role-specific keep-recent/interval already set above.
		// For everything/nothing/default the SDK ignores keep-recent/interval.
	}

	for k, v := range appMap {
		if err := nc.Set(nc.App, k, v); err != nil {
			return err
		}
	}
	return nil
}

// applyClientTuning ensures client.toml uses the file keyring backend and the
// configured chain-id, so umeshd finds keys created by umeshctl via
// `umeshd keys add --keyring-backend file`.
func applyClientTuning(nc *nodeconfig.NodeConfig, opt Options) error {
	if nc.Client == nil {
		return nil
	}
	if err := nc.Set(nc.Client, "keyring-backend", KeyringBackend); err != nil {
		return err
	}
	chainID := opt.ChainID
	if chainID != "" {
		if err := nc.Set(nc.Client, "chain-id", chainID); err != nil {
			return err
		}
	}
	return nil
}
