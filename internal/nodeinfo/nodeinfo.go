// Package nodeinfo persists node metadata to config/.node-info.
//
// The file is a JSON document describing how a node was initialized (its role,
// chain-id, validator addresses, etc.) and is read by umeshctl init, tune and
// verify to decide which profile applies and whether the node is ready.
package nodeinfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Info is the on-disk representation of a node's initialization state.
type Info struct {
	ChainID            string    `json:"chain_id"`
	Mode               string    `json:"mode"`
	ValidatorOperator  string    `json:"validator_operator"`
	ValidatorAddress   string    `json:"validator_address"`
	NodeID             string    `json:"node_id"`
	KeyringBackend     string    `json:"keyring_backend"`
	GenesisTime        time.Time `json:"genesis_time"`
	AutoAccountCreated bool      `json:"auto_account_created"`
	ValidatorReady     int       `json:"VALIDATOR_READY"`
	GenesisSHA256      string    `json:"genesis_sha256,omitempty"`
	CLIVersion         string    `json:"cli_version,omitempty"`
	InitDate           time.Time `json:"init_date,omitempty"`
}

// Path returns the on-disk location of .node-info for a given home directory.
func Path(homeDir string) string {
	return filepath.Join(homeDir, "config", ".node-info")
}

var cliVersion = "dev"

func SetCLIVersion(v string) { cliVersion = v }

// Write persists info as JSON to home/.node-info. Parent directories are
// created if missing.
func Write(homeDir string, info Info) error {
	if info.InitDate.IsZero() {
		info.InitDate = time.Now()
	}
	info.CLIVersion = cliVersion
	p := Path(homeDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// Read loads Info from home/.node-info. Returns an error if the file is
// missing or malformed.
func Read(homeDir string) (*Info, error) {
	data, err := os.ReadFile(Path(homeDir))
	if err != nil {
		return nil, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
