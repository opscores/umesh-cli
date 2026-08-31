// Package backup copies node keys and configuration out of the container into
// a timestamped backup directory, mirroring backup-keys.sh.
package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opscores/umesh-cli/internal/dkrcmd"
)

// FilesForRole returns the in-container files to back up for a role.
func FilesForRole(role string) []string {
	config := "/home/umesh/.umeshnode/config"
	data := "/home/umesh/.umeshnode/data"
	if role == "sentry" {
		return []string{
			config + "/node_key.json",
			config + "/genesis.json",
		}
	}
	// validator / genesis / sentry / rpc all keep the same critical set
	// (RPC auto-generates a disposable priv_validator_key; backing it up is harmless).
	return []string{
		config + "/priv_validator_key.json",
		config + "/node_key.json",
		config + "/genesis.json",
		data + "/priv_validator_state.json",
	}
}

// Run backs up files from a running container (docker exec cat) into
// host/backupDir, chmod 0600. Returns the backup directory path.
func Run(d *dkrcmd.Docker, role, backupDir string) (string, error) {
	ts := time.Now().Format("20060102-150405")
	dir := filepath.Join(backupDir, "backup-"+ts)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	copied := 0
	missing := 0
	for _, f := range FilesForRole(role) {
		out, err := d.ExecOutput("cat", f)
		if err != nil {
			missing++
			continue
		}
		dst := filepath.Join(dir, filepath.Base(f))
		if err := os.WriteFile(dst, []byte(out+"\n"), 0o600); err != nil {
			return "", fmt.Errorf("write backup: %w", err)
		}
		copied++
	}
	if copied == 0 {
		return "", fmt.Errorf("no files backed up (container may be stopped or files missing)")
	}
	if missing > 0 {
		return "", fmt.Errorf("backup incomplete: %d file(s) missing", missing)
	}
	return dir, nil
}
