package nodeinit

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/opscores/umesh-cli/internal/nodeconfig"
	"github.com/opscores/umesh-cli/internal/nodeinfo"
	"github.com/opscores/umesh-cli/internal/tune"
	"github.com/opscores/umesh-cli/internal/uio"
)

// SentryParams carries env values for the init-sentry flow.
type SentryParams struct {
	ChainID         string
	Moniker         string
	Denom           string
	MinGasPrice     string
	Environment     string
	SentryRPC       string
	ValidatorRPC    string
	GenesisURL      string
	PublicFallback  string
	GenesisSHA256   string
	AddrbookURL     string
	AddrbookSHA256  string
	Seeds           string
	PersistentPeers string
	ExternalIP      string
	PublicIP        string
	ExternalPort    string
	UsePrivate      bool
}

// RunSentry executes the sentry node init flow. A sentry is started as a
// peer-facing relay; on the private flag it also registers the validator as a
// private peer. Idempotent: returns ErrAlreadyInitialized if genesis.json
// already exists.
func RunSentry(p SentryParams) error {
	if err := AbortIfInitialized(); err != nil {
		return err
	}
	for _, ip := range []string{p.ExternalIP, p.PublicIP} {
		if ip != "" {
			if err := ValidatePrivateIP(ip, false); err != nil {
				return err
			}
		}
	}

	d := docker()
	if err := d.Preflight(); err != nil {
		return err
	}

	// Fetch genesis and extract chain-id/denom BEFORE validation and umeshd init,
	// so that --chain-id is available and auto-fill can satisfy ValidateCommon.
	res, err := obtainGenesis(p.GenesisURL, p.SentryRPC, p.ValidatorRPC, p.PublicFallback, p.GenesisSHA256, p.ChainID, p.Denom)
	if err != nil {
		return err
	}
	if p.Denom == "" {
		p.Denom = res.BondDenom
	}
	if p.ChainID == "" {
		p.ChainID = res.ChainID
	}
	if err := ValidateCommon(p.ChainID, p.Moniker, p.Denom, p.MinGasPrice, p.Environment); err != nil {
		return err
	}

	// Sentry never signs blocks. Drop any stale consensus state or signing
	// key so the node boots cleanly on reinitialisation.
	if err := umeshdInit(d, p.Moniker, p.ChainID, Home(), true); err != nil {
		return err
	}

	// Write the downloaded genesis.json (obtained before init) over the dummy genesis created by umeshd init
	genesisPath := filepath.Join(ConfigDir(), "genesis.json")
	if err := os.WriteFile(genesisPath, res.RawData, 0o644); err != nil {
		return err
	}

	if err := validateGenesis(d); err != nil {
		return err
	}
	if err := tune.Apply(ConfigDir(), tune.RoleSentry, tune.Options{Moniker: p.Moniker, Environment: p.Environment, Denom: p.Denom, MinGasPrice: p.MinGasPrice, ExternalAddress: p.ExternalIP, ChainID: p.ChainID}); err != nil {
		return err
	}

	if p.AddrbookURL != "" {
		if err := downloadAddrbook(p.AddrbookURL, p.AddrbookSHA256); err != nil {
			return err
		}
	}

	if p.Seeds != "" || p.PersistentPeers != "" {
		if err := applyP2P(p.Seeds, p.PersistentPeers); err != nil {
			return err
		}
	}

	if p.ExternalIP != "" {
		if err := setExternalAddress(joinHostPort(p.ExternalIP, p.ExternalPort)); err != nil {
			return err
		}
	}

	if p.UsePrivate {
		valID, err := nodeIDFromRPC(p.ValidatorRPC)
		if err == nil && valID != "" {
			if err := addPrivatePeer(valID); err != nil {
				return err
			}
		} else {
			// Best-effort: do not fail init, but guide operator to offline fallback.
			if p.ValidatorRPC == "" {
				uio.LogWarning("usePrivate=true but validatorRpc is empty — cannot fetch validator NodeID; set network.persistentPeers to '<nodeID>@<validator_ip>:26656' manually (NodeID from: umeshd show-node-id --home ./data-validator or data-validator/config/node_key.json)")
			} else {
				uio.LogWarning("could not fetch validator NodeID from %q: %v — init continues without private_peer_ids; set network.persistentPeers to '<nodeID>@<ip>:26656' manually (NodeID from: cat ./data-validator/config/node_key.json or umeshd show-node-id --home ./data-validator)", p.ValidatorRPC, err)
			}
		}
	}

	info := nodeinfo.Info{
		ChainID:            p.ChainID,
		Mode:               "sentry",
		ValidatorOperator:  "",
		ValidatorAddress:   "",
		NodeID:             localNodeID(Home()),
		KeyringBackend:     "file",
		GenesisTime:        extractGenesisTime(GenesisFile()),
		AutoAccountCreated: false,
		ValidatorReady:     0,
	}
	return nodeinfo.Write(Home(), info)
}

// addPrivatePeer appends a node ID to unconditional/private peer lists.
func addPrivatePeer(nodeID string) error {
	nc, err := nodeconfig.Load(ConfigDir())
	if err != nil {
		return err
	}
	for _, key := range []string{"p2p.unconditional_peer_ids", "p2p.private_peer_ids"} {
		cur := nc.Config.GetString(key, "")
		if merged := nodeconfig.MergePeerID(cur, nodeID); merged != cur {
			if err := nc.Set(nc.Config, key, merged); err != nil {
				return err
			}
		}
	}
	return nil
}

// nodeIDFromRPC fetches the node ID from a RPC /status endpoint.
func nodeIDFromRPC(rpc string) (string, error) {
	body, err := httpGet(rpc + "/status")
	if err != nil {
		return "", err
	}
	var env struct {
		Result struct {
			NodeInfo struct {
				ID string `json:"id"`
			} `json:"node_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", err
	}
	return env.Result.NodeInfo.ID, nil
}
