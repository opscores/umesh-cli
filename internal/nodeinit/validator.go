package nodeinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeconfig"
	"github.com/opscores/umesh-cli/internal/nodeinfo"
	"github.com/opscores/umesh-cli/internal/tune"
	"github.com/opscores/umesh-cli/internal/uio"
)

// ValidatorParams carries the env values for the validator (join existing
// network) init flow.
type ValidatorParams struct {
	ChainID         string
	Moniker         string
	Denom           string
	MinGasPrice     string
	Environment     string
	KeyringPass     string
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
}

// RunValidator executes the validator init flow: joining an existing network
// as a post-genesis validator. Runs on the host via docker run --rm.
// Idempotent: returns ErrAlreadyInitialized if genesis.json already exists.
func RunValidator(p ValidatorParams) error {
	if err := AbortIfInitialized(); err != nil {
		return err
	}
	if p.KeyringPass == "" {
		return fmt.Errorf("keyring password is required (use --keyring-password-file, --keyring-password-stdin, or --keyring-password-exec)")
	}

	d := docker()
	if err := d.Preflight(); err != nil {
		return err
	}

	// Fetch genesis and extract chain-id/denom BEFORE validation and umeshd init,
	// so that --chain-id is available for the init command and auto-fill can
	// satisfy ValidateCommon when chainId/denom are omitted in YAML (join mode).
	res, err := obtainGenesis(p.GenesisURL, p.SentryRPC, p.ValidatorRPC, p.PublicFallback, p.GenesisSHA256, p.ChainID, p.Denom)
	if err != nil {
		return err
	}
	// Auto-fill denom and chain-id from genesis if not provided in config
	if p.Denom == "" {
		p.Denom = res.BondDenom
	}
	if p.ChainID == "" {
		p.ChainID = res.ChainID
	}
	if err := ValidateCommon(p.ChainID, p.Moniker, p.Denom, p.MinGasPrice, p.Environment); err != nil {
		return err
	}

	if err := umeshdInit(d, p.Moniker, p.ChainID, Home(), true); err != nil {
		return err
	}

	// Write the downloaded genesis.json (obtained before init) over the dummy genesis created by umeshd init
	genesisPath := filepath.Join(ConfigDir(), "genesis.json")
	if err := os.WriteFile(genesisPath, res.RawData, 0o644); err != nil {
		return err
	}

	// Step 4.1: Auto-backup consensus keys immediately after genesis init.
	// Backs up priv_validator_key.json (consensus key) and node_key.json (P2P key).
	// Note: priv_validator_state.json is also backed up at height=0 (initial state).
	// Restoring priv_validator_state.json on a LIVE validator will cause
	// double-signing → permanent jail/tombstone. Never restore on a running node.
	if err := backupConsensusKeysStep(ConfigDir(), BackupsDir(Home())); err != nil {
		uio.LogWarning("Auto-backup of consensus keys failed: %v", err)
	}

	if err := validateGenesis(d); err != nil {
		return err
	}
	if err := tune.Apply(ConfigDir(), tune.RoleValidator, tune.Options{Moniker: p.Moniker, Environment: p.Environment, Denom: p.Denom, MinGasPrice: p.MinGasPrice, ExternalAddress: p.ExternalIP, ChainID: p.ChainID}); err != nil {
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
		if err := ValidatePrivateIP(p.ExternalIP, false); err != nil {
			return err
		}
		if err := setExternalAddress(p.ExternalIP); err != nil {
			return err
		}
	} else if p.Environment == "production" || p.Environment == "" {
		// Offline init: external_address not set → CometBFT will advertise Docker bridge IP (172.x).
		// Validator has pex=false but sentry still needs persistent_peers to dial it.
		uio.LogWarning("externalAddress not set — CometBFT will advertise Docker bridge IP (172.x) which peers cannot dial; set network.externalAddress to public IP for production")
	}

	autoKey := false
	if _, err := ensureKey(d, "validator", p.KeyringPass); err == nil {
		autoKey = true
	}

	info := nodeinfo.Info{
		ChainID:            p.ChainID,
		Mode:               "validator",
		ValidatorOperator:  valoperFromKeyring(d, "validator", p.KeyringPass),
		ValidatorAddress:   localValidatorAddr(Home()),
		NodeID:             localNodeID(Home()),
		KeyringBackend:     "file",
		GenesisTime:        extractGenesisTime(GenesisFile()),
		AutoAccountCreated: autoKey,
		ValidatorReady:     0,
	}
	return nodeinfo.Write(Home(), info)
}

// obtainGenesis downloads genesis from the configured source chain.
// Tries sources in priority order: sentry RPC, validator RPC, direct URL,
// fallback URL. Returns extracted network parameters (chain-id, bond_denom)
// with RawData for later writing after umeshd init.
func obtainGenesis(genesisURL, sentryRPC, validatorRPC, fallback, wantSHA, chainID, denom string) (*FetchGenesisResult, error) {
	sources := []string{
		sentryRPC + "/genesis",
		validatorRPC + "/genesis",
		genesisURL,
		fallback + "/genesis",
	}
	type attempt struct {
		URL string
		Err error
	}
	var attempts []attempt
	for _, url := range sources {
		if url == "/genesis" || url == "" {
			continue
		}
		res, err := FetchGenesis(FetchGenesisParams{
			URL:       url,
			SHA256:    wantSHA,
			ChainID:   chainID,
			Denom:     denom,
			WriteFile: false, // We'll write after umeshd init
		})
		if err == nil {
			return res, nil
		}
		attempts = append(attempts, attempt{URL: url, Err: err})
	}
	if len(attempts) == 0 {
		return nil, fmt.Errorf("failed to download genesis from any source (set join.genesisUrl / join.sentryRpc in config, or use --genesis-url / --sentry-rpc flags)")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "failed to download genesis from %d source(s) (set join.genesisUrl / join.sentryRpc in config, or use --genesis-url / --sentry-rpc flags):", len(attempts))
	for _, a := range attempts {
		fmt.Fprintf(&sb, "\n  - %s: %v", a.URL, a.Err)
	}
	sb.WriteString("\nHint: verify with umeshctl genesis fetch --url <url> --dry-run")
	return nil, fmt.Errorf("%s", sb.String())
}

func downloadAddrbook(url, wantSHA string) error {
	body, err := DownloadGenesis(url, 3, 5, true)
	if err != nil {
		return err
	}
	if wantSHA != "" {
		if err := VerifyHash(wantSHA, SHA256Hex(body)); err != nil {
			return err
		}
	}
	return os.WriteFile(strings.Join([]string{ConfigDir(), "addrbook.json"}, "/"), body, 0o644)
}

// applyP2P writes seeds/persistent_peers into config.toml.
func applyP2P(seeds, persistentPeers string) error {
	nc, err := nodeconfig.Load(ConfigDir())
	if err != nil {
		return err
	}
	if err := nc.Set(nc.Config, "p2p.seeds", seeds); err != nil {
		return err
	}
	return nc.Set(nc.Config, "p2p.persistent_peers", persistentPeers)
}

// ensureKey creates (or reuses) a keyring key and returns its JSON.
func ensureKey(d *dkrcmd.Docker, name, password string) (string, error) {
	out, err := keysShow(d, name, password)
	if err == nil && strings.Contains(out, `"name":"`+name+`"`) {
		return out, nil
	}
	out, err = keysAdd(d, name, password)
	if err != nil {
		return "", fmt.Errorf("keyring creation failed: %w", err)
	}
	return out, nil
}

// backupConsensusKeysStep copies priv_validator_key.json, node_key.json, and
// priv_validator_state.json from the node's config dir to a role-specific
// backup directory with chmod 600 for security.
func backupConsensusKeysStep(configDir, backupDir string) error {
	ts := time.Now().Format("20060102-150405")
	dest := filepath.Join(backupDir, "validator-consensus-"+ts)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	files := []string{"priv_validator_key.json", "node_key.json", "priv_validator_state.json"}
	for _, f := range files {
		src := filepath.Join(configDir, f)
		data, err := os.ReadFile(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read %s: %w", src, err)
		}
		dst := filepath.Join(dest, f)
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}
	return nil
}
