package nodeinit

import (
	"os"
	"path/filepath"

	"github.com/opscores/umesh-cli/internal/nodeconfig"
	"github.com/opscores/umesh-cli/internal/nodeinfo"
	"github.com/opscores/umesh-cli/internal/tune"
)

// RPCParams carries env values for the init-rpc flow.
type RPCParams struct {
	ChainID         string
	Moniker         string
	Denom           string
	MinGasPrice     string
	Environment     string
	Pruning         string
	RPCUpstream     string
	RESTUpstream    string
	P2PUpstream     string
	SentryRPC       string
	GenesisURL      string
	GenesisSHA256   string
	ValidatorRPC    string
	Seeds           string
	PersistentPeers string
	ExternalIP      string
}

// RunRPC executes the lightweight public RPC node init flow. Idempotent:
// returns ErrAlreadyInitialized if genesis.json already exists.
func RunRPC(p RPCParams) error {
	if err := AbortIfInitialized(); err != nil {
		return err
	}

	d := docker()
	if err := d.Preflight(); err != nil {
		return err
	}

	// Fetch genesis and extract chain-id/denom BEFORE validation and umeshd init.
	// Priority for RPC: sentryRpc (RPC) → genesisUrl (static) → validatorRpc (RPC fallback).
	// obtainGenesis order is sentry+/genesis, validator+/genesis, genesisUrl, fallback+/genesis,
	// so we map genesisUrl to the 3rd slot and validatorRpc to fallback to achieve the desired order.
	res, err := obtainGenesis(p.GenesisURL, p.SentryRPC, "", p.ValidatorRPC, p.GenesisSHA256, p.ChainID, p.Denom)
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

	// RPC never signs blocks. Drop any stale consensus state or signing key
	// so the node boots cleanly on reinitialisation.
	if err := umeshdInit(d, p.Moniker, p.ChainID, Home(), true); err != nil {
		return err
	}

	// Write the downloaded genesis.json (obtained before init) over the dummy genesis created by umeshd init
	genesisPath := filepath.Join(ConfigDir(), "genesis.json")
	if err := os.WriteFile(genesisPath, res.RawData, 0o644); err != nil {
		return err
	}

	if err := enableRPC(); err != nil {
		return err
	}
	if err := tune.Apply(ConfigDir(), tune.RoleRPC, tune.Options{Moniker: p.Moniker, Environment: p.Environment, Denom: p.Denom, MinGasPrice: p.MinGasPrice, ExternalAddress: p.ExternalIP, ChainID: p.ChainID, Pruning: p.Pruning}); err != nil {
		return err
	}

	if p.Seeds != "" || p.PersistentPeers != "" {
		if err := applyP2P(p.Seeds, p.PersistentPeers); err != nil {
			return err
		}
	}

	if p.ExternalIP != "" {
		if err := ValidatePrivateIP(p.ExternalIP, false); err != nil {
			return err
		}
		if err := setExternalAddress(p.ExternalIP); err != nil {
			return err
		}
	}

	info := nodeinfo.Info{
		ChainID:            p.ChainID,
		Mode:               "rpc",
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

// enableRPC opens the tendermint RPC and REST endpoints for public exposure.
func enableRPC() error {
	nc, err := nodeconfig.Load(ConfigDir())
	if err != nil {
		return err
	}
	if err := nc.Set(nc.Config, "rpc.laddr", "tcp://0.0.0.0:26657"); err != nil {
		return err
	}
	if err := nc.Set(nc.Config, "rpc.cors_allowed_origins", []string{"*"}); err != nil {
		return err
	}
	return nil
}
