package nodeinit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opscores/umesh-cli/internal/dkrcmd"
)

func osStat(p string) (os.FileInfo, error) {
	return os.Stat(p)
}

// keysShow shows a key from the keyring via docker run.
func keysShow(d *dkrcmd.Docker, name, password string) (string, error) {
	out, err := d.RunMount(strings.NewReader(password+"\n"), "keys", "show", name,
		"--keyring-backend", "file", "--keyring-dir", containerHome(d)+"/keyring",
		"--home", containerHome(d), "--output", "json")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// keysAdd adds a key to the keyring via docker run.
func keysAdd(d *dkrcmd.Docker, name, password string) (string, error) {
	out, err := d.RunMount(strings.NewReader(password+"\n"+password+"\n"), "keys", "add", name,
		"--keyring-backend", "file", "--keyring-dir", containerHome(d)+"/keyring",
		"--home", containerHome(d), "--output", "json")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// validateGenesis runs `umeshd genesis validate-genesis` via docker run.
func validateGenesis(d *dkrcmd.Docker) error {
	_, err := d.RunMount(nil, "genesis", "validate-genesis", "--home", containerHome(d))
	return err
}

// localNodeID returns the p2p node ID for the home dir via a temporary container.
func localNodeID(home string) string {
	d := dkrcmd.New(
		dkrcmd.WithDataDir(home),
		dkrcmd.WithBackupsDir(BackupsDir(home)),
		dkrcmd.WithHome("/home/umesh/.umeshnode"),
		dkrcmd.WithNetwork("umesh"),
		dkrcmd.WithImage(nodeImage()),
	)
	out, err := d.RunMount(nil, "comet", "show-node-id", "--home", containerHome(d))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// localValidatorAddr reads the validator address from priv_validator_key.json.
func localValidatorAddr(home string) string {
	path := filepath.Join(home, "config", "priv_validator_key.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var kv struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(data, &kv); err != nil {
		return ""
	}
	return kv.Address
}

// extractGenesisTime parses genesis_time from a genesis.json file.
func extractGenesisTime(genesisPath string) time.Time {
	data, err := os.ReadFile(genesisPath)
	if err != nil {
		return time.Time{}
	}
	var g struct {
		GenesisTime string `json:"genesis_time"`
	}
	if err := json.Unmarshal(data, &g); err != nil {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, g.GenesisTime)
	return t
}

// valoperFromKeyring returns the validator operator address (umeshvaloper1...)
// for the named key via `umeshd keys show --bech val`.
func valoperFromKeyring(d *dkrcmd.Docker, name, password string) string {
	out, err := d.RunMount(strings.NewReader(password+"\n"),
		"keys", "show", name,
		"--bech", "val",
		"--keyring-backend", "file",
		"--keyring-dir", containerHome(d)+"/keyring",
		"--home", containerHome(d),
		"--output", "json")
	if err != nil {
		return ""
	}
	var k struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(out), &k); err != nil {
		return ""
	}
	return k.Address
}

// LocalNodeID returns the p2p node ID for the home dir via a temporary container.
// Exported for use by cmd package (restore, doctor).
func LocalNodeID(home string) string {
	return localNodeID(home)
}

// ExtractGenesisTime parses genesis_time from a genesis.json file.
// Exported for use by cmd package (restore, doctor).
func ExtractGenesisTime(genesisPath string) time.Time {
	return extractGenesisTime(genesisPath)
}

// ExtractChainID extracts the chain_id from a genesis.json file.
// Exported for use by cmd package (restore).
func ExtractChainID(genesisPath string) string {
	data, err := os.ReadFile(genesisPath)
	if err != nil {
		return ""
	}
	var g struct {
		ChainID string `json:"chain_id"`
	}
	if err := json.Unmarshal(data, &g); err != nil {
		return ""
	}
	return g.ChainID
}
