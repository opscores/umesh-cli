// Package nodeinit implements the four role initialization flows
// (genesis/validator/sentry/rpc).
package nodeinit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opscores/umesh-cli/internal/dkrcmd"
)

// privValidatorState is the on-disk shape of data/data/priv_validator_state.json.
type privValidatorState struct {
	Height string `json:"height"`
	Round  int    `json:"round"`
	Step   int    `json:"step"`
}

// resetValidatorState resets or removes the CometBFT validator signing state.
// When reinitialising a node (genesis/join --force) a stale state with height>0
// causes CometBFT v0.39 to panic with "error replaying blocks". We either zero it
// out (genesis, where the chain restarts at height 1) or delete it entirely
// (join, where the node syncs from peers and must not carry local consensus state).
func resetValidatorState(homeDir string, remove bool) error {
	p := filepath.Join(homeDir, "data", "priv_validator_state.json")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	}
	if remove {
		return os.Remove(p)
	}
	st := privValidatorState{Height: "0", Round: 0, Step: 0}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal priv_validator_state: %w", err)
	}
	return os.WriteFile(p, data, 0o600)
}

var currentHome string

// homeCandidates returns data directory paths in detection order.
func homeCandidates() []string {
	if env := os.Getenv("UMESH_HOME"); env != "" {
		return []string{env}
	}
	return []string{
		"./data-validator",
		"./data-sentry",
		"./data-rpc",
	}
}

// DetectHome returns the first existing data directory with .node-info,
// or the first existing data directory, or "./data-validator" as fallback.
func DetectHome() string {
	for _, dir := range homeCandidates() {
		if _, err := os.Stat(filepath.Join(dir, "config", ".node-info")); err == nil {
			return dir
		}
	}
	for _, dir := range homeCandidates() {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return "./data-validator"
}

// SetHome sets the active node home directory (overrides auto-detection).
func SetHome(h string) {
	if h != "" {
		currentHome = h
	}
}

// Home returns the node home directory (auto-detects on first call).
func Home() string {
	if currentHome == "" {
		currentHome = DetectHome()
	}
	return currentHome
}

// ConfigDir returns the config directory path.
func ConfigDir() string { return filepath.Join(Home(), "config") }

// GenesisFile returns the path to genesis.json.
func GenesisFile() string { return filepath.Join(ConfigDir(), "genesis.json") }

// GenesisExists reports whether genesis.json is present.
func GenesisExists() bool { return fileExists(GenesisFile()) }

// ErrAlreadyInitialized signals that a node has already been initialized and
// the init flow should be skipped. Callers may intercept it to emit a warning
// rather than a failure.
var ErrAlreadyInitialized = errors.New("node already initialized")

// ForceReinit, when true, causes AbortIfInitialized to return nil even when
// genesis.json exists. Set by the init command when --force is passed.
var ForceReinit bool

// KeepKeys, when true, preserves the node identity files (CometBFT consensus
// signing key and P2P node key) across a `--force` re-initialisation, and
// resets the blockchain state so a regenerated genesis can start cleanly.
// Set by the plan command when --keep-keys is passed.
var KeepKeys bool

// AbortIfInitialized returns ErrAlreadyInitialized when genesis.json is present,
// indicating the init flow should not run again. If ForceReinit is true, it
// returns nil regardless.
func AbortIfInitialized() error {
	if ForceReinit {
		return nil
	}
	if GenesisExists() {
		return fmt.Errorf("%w: %s already exists", ErrAlreadyInitialized, GenesisFile())
	}
	return nil
}

// nodeImage returns the container image used for offline operations, honoring
// the NODE_IMAGE environment variable (the same source as docker-compose.yml).
func nodeImage() string {
	if v := strings.TrimSpace(os.Getenv("NODE_IMAGE")); v != "" {
		return v
	}
	return "umesh-node:latest"
}

// docker returns a dkrcmd.Docker configured for offline host-side operations.
func docker() *dkrcmd.Docker {
	return dkrcmd.New(
		dkrcmd.WithDataDir(Home()),
		dkrcmd.WithBackupsDir(BackupsDir(Home())),
		dkrcmd.WithHome("/home/umesh/.umeshnode"),
		dkrcmd.WithNetwork("umesh"),
		dkrcmd.WithImage(nodeImage()),
	)
}

// KeyringConfigDir returns the directory for keyring password files.
// Uses XDG_CONFIG_HOME/umesh or ~/.config/umesh as fallback.
func KeyringConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "umesh")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "umesh")
}

// BackupsDir mirrors the role-specific host backup dir used by docker-compose:
// ./backups-validator, ./backups-sentry, ./backups-rpc. The same data dir
// naming is shared so that the offline docker run and the running compose
// container agree on the host path. Exported so cmd/ can reuse it.
func BackupsDir(home string) string {
	switch {
	case strings.HasSuffix(home, "data-sentry"):
		return "./backups-sentry"
	case strings.HasSuffix(home, "data-rpc"):
		return "./backups-rpc"
	default:
		return "./backups-validator"
	}
}

// umeshdInit runs `umeshd init <moniker> --chain-id <id>` on the host via docker.
// When ForceReinit is set the stale CometBFT signing state is reset so that a
// node previously run at a higher height does not panic with
// "error replaying blocks" on the fresh chain.
//
// removeState selects the reset policy:
//   - genesis: zero state (height 0) — the chain restarts at block 1
//   - validator/sentry/rpc: delete state — the node syncs from peers and must
//     not carry local consensus state; sentry/rpc also have no signing key
//
// When KeepKeys is set together with ForceReinit the node identity files
// (node_key.json + priv_validator_key.json) are captured before init and
// restored afterwards so a sentry re-init does not change its NodeID.
func umeshdInit(d *dkrcmd.Docker, moniker, chainID, homeDir string, removeState bool) error {
	var keyBackup *identityKeys
	if ForceReinit && KeepKeys {
		kb, err := captureIdentityKeys(homeDir)
		if err != nil {
			return fmt.Errorf("capture node keys failed: %w", err)
		}
		keyBackup = kb
	}
	if ForceReinit {
		// Offline operation: must not run while container is live — would cause
		// double-sign risk (stale priv_validator_state) and DB lock.
		if d.IsRunning() {
			return fmt.Errorf("refusing --force while container %q is running — stop it first (docker compose down or docker stop %s)", d.Container, d.Container)
		}
		if err := resetValidatorState(homeDir, removeState); err != nil {
			return err
		}
		// Drop the consensus signing key so a reinit cannot carry a key from a
		// prior run. Required for sentry/rpc (no key), safe for join; for
		// genesis the key is regenerated in createValidatorAccount below.
		// When KeepKeys is set the key will be restored after init, so skip deletion.
		if removeState && !KeepKeys {
			if err := removeConsensusKeys(homeDir); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	args := []string{"init", moniker, "--chain-id", chainID, "--home", d.Home}
	if ForceReinit {
		args = append(args, "--overwrite")
	}
	_, err := d.RunMount(nil, args...)
	if err != nil {
		return err
	}
	if ForceReinit && KeepKeys && keyBackup != nil {
		if err := keyBackup.restore(homeDir); err != nil {
			return fmt.Errorf("restore node keys failed: %w", err)
		}
	}
	return nil
}

// privValidatorKeyFile returns the path to the consensus signing key for a home dir.
func privValidatorKeyFile(homeDir string) string {
	return filepath.Join(homeDir, "config", "priv_validator_key.json")
}

// removeConsensusKeys deletes priv_validator_key.json so a signing key is
// regenerated and the node cannot double-sign across reinitialisations.
func removeConsensusKeys(homeDir string) error {
	return os.Remove(privValidatorKeyFile(homeDir))
}

// identityKeys is an in-memory backup of the node identity files that
// `umeshd init --overwrite` would otherwise regenerate: the CometBFT consensus
// signing key and the P2P node key.
type identityKeys struct {
	consensusKey []byte
	nodeKey      []byte
}

// nodeKeyFile returns the path to the P2P node key for a home dir.
func nodeKeyFile(homeDir string) string {
	return filepath.Join(homeDir, "config", "node_key.json")
}

// readOptional reads a file, returning nil when it does not exist.
func readOptional(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

// captureIdentityKeys reads the node identity files before a re-initialisation
// so they can be restored afterwards (KeepKeys mode).
func captureIdentityKeys(homeDir string) (*identityKeys, error) {
	ik := &identityKeys{}
	var err error
	if ik.consensusKey, err = readOptional(privValidatorKeyFile(homeDir)); err != nil {
		return nil, fmt.Errorf("read %s: %w", privValidatorKeyFile(homeDir), err)
	}
	if ik.nodeKey, err = readOptional(nodeKeyFile(homeDir)); err != nil {
		return nil, fmt.Errorf("read %s: %w", nodeKeyFile(homeDir), err)
	}
	return ik, nil
}

// restore writes the captured identity files back after a re-initialisation.
// Files that did not exist before are not recreated.
func (ik *identityKeys) restore(homeDir string) error {
	if ik == nil {
		return nil
	}
	if len(ik.consensusKey) > 0 {
		if err := os.WriteFile(privValidatorKeyFile(homeDir), ik.consensusKey, 0o600); err != nil {
			return fmt.Errorf("restore priv_validator_key.json: %w", err)
		}
	}
	if len(ik.nodeKey) > 0 {
		if err := os.WriteFile(nodeKeyFile(homeDir), ik.nodeKey, 0o600); err != nil {
			return fmt.Errorf("restore node_key.json: %w", err)
		}
	}
	return nil
}

// resetChainState removes the blockchain databases, WAL, and snapshots left by
// a previous run so a regenerated genesis starts at height 1 with a clean
// store. The keyring and node identity files are intentionally preserved.
// priv_validator_state.json is reset separately by resetValidatorState.
func resetChainState(homeDir string) error {
	dataDir := filepath.Join(homeDir, "data")
	for _, name := range []string{
		"blockstore.db", "state.db", "application.db",
		"evidence.db", "tx_index.db", "cs.wal", "snapshots",
	} {
		p := filepath.Join(dataDir, name)
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("reset %s: %w", name, err)
		}
	}
	return nil
}
