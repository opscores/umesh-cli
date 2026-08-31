package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeinfo"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newRestoreCmd() *cobra.Command {
	var from, role string
	var yes, dryRun bool

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore node from backup",
		Long: `Restore node keys and configuration from a backup directory.

WARNING: This will replace existing keys. Make sure the node is stopped.

  umeshctl ops restore --from ./backups/20260810 --role validator
  umeshctl ops restore --from ./backups/20260810 --role sentry
  umeshctl ops restore --from ./backups/20260810 --role validator --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" {
				return fmt.Errorf("--from is required")
			}
			if role == "" {
				return fmt.Errorf("--role is required")
			}

			if dryRun {
				uio.LogInfo("DRY RUN: would restore %s from %s", role, from)
				uio.LogInfo("DRY RUN: would restore genesis.json, keys, and keyring")
				return nil
			}

			// Verify backup exists
			if _, err := os.Stat(from); os.IsNotExist(err) {
				return fmt.Errorf("backup not found: %s", from)
			}

			uio.LogWarning("This will replace existing keys for role %s", role)
			uio.LogInfo("Ensure the node is stopped before restoring")

			// Irreversible: require explicit confirmation on a TTY.
			ok, err := uio.Confirm(fmt.Sprintf("Replace existing keys for role %q from %s? This is irreversible", role, from), yes)
			if err != nil {
				return err
			}
			if !ok {
				uio.LogInfo("Aborted.")
				return nil
			}
			// Offline phase: host file copy — warn if container is live (risk of overwriting active keys).
			if dkrcmd.New(dkrcmd.WithContainer(global.Container)).IsRunning() {
				uio.LogWarning("container %q is running — stop it first to avoid corrupting live keys", global.Container)
			}

			home := nodeinit.Home()
			configDir := filepath.Join(home, "config")

			// Restore genesis.json from backup (if present)
			genesisSrc := filepath.Join(from, "genesis.json")
			genesisDst := filepath.Join(configDir, "genesis.json")
			if _, err := os.Stat(genesisSrc); err == nil {
				if err := copyFileRestore(genesisSrc, genesisDst); err != nil {
					return fmt.Errorf("restore genesis.json: %w", err)
				}
				uio.LogSuccess("Restored genesis.json")
			}

			// Restore based on role
			switch role {
			case "validator":
				if err := restoreValidator(from, home, configDir); err != nil {
					return err
				}
			case "sentry", "rpc":
				if err := restoreNonValidator(from, configDir); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown role: %s", role)
			}

			// Write .node-info with restored metadata
			var chainID string
			genesisPath := filepath.Join(configDir, "genesis.json")
			if data, err := os.ReadFile(genesisPath); err == nil {
				var g struct {
					ChainID string `json:"chain_id"`
				}
				if json.Unmarshal(data, &g) == nil {
					chainID = g.ChainID
				}
			}
			info := nodeinfo.Info{
				Mode:           role,
				ChainID:        chainID,
				NodeID:         nodeinit.LocalNodeID(home),
				KeyringBackend: "file",
				GenesisTime:    nodeinit.ExtractGenesisTime(genesisPath),
				ValidatorReady: 0,
			}
			if err := nodeinfo.Write(home, info); err != nil {
				uio.LogWarning("Failed to write .node-info: %v", err)
			} else {
				uio.LogSuccess("Updated .node-info (role=%s, chain_id=%s)", role, chainID)
			}

			uio.LogSuccess("Restore completed from %s", from)
			return nil
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Path to backup directory")
	cmd.Flags().StringVar(&role, "role", "", "Node role (validator/sentry/rpc)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be restored without executing")
	_ = cmd.RegisterFlagCompletionFunc("role", completeRoles())
	return cmd
}

func restoreValidator(from, home, configDir string) error {
	// Restore priv_validator_key.json
	srcKey := filepath.Join(from, "priv_validator_key.json")
	dstKey := filepath.Join(configDir, "priv_validator_key.json")
	if err := copyFileRestore(srcKey, dstKey); err != nil {
		return fmt.Errorf("restore priv_validator_key.json: %w", err)
	}
	uio.LogSuccess("Restored priv_validator_key.json")

	// Restore node_key.json
	srcNode := filepath.Join(from, "node_key.json")
	dstNode := filepath.Join(configDir, "node_key.json")
	if err := copyFileRestore(srcNode, dstNode); err != nil {
		return fmt.Errorf("restore node_key.json: %w", err)
	}
	uio.LogSuccess("Restored node_key.json")

	// Restore keyring
	srcKeyring := filepath.Join(from, "keyring-file")
	dstKeyring := filepath.Join(home, "keyring", "keyring-file")
	if _, err := os.Stat(srcKeyring); err == nil {
		if err := copyDirRestore(srcKeyring, dstKeyring); err != nil {
			return fmt.Errorf("restore keyring: %w", err)
		}
		uio.LogSuccess("Restored keyring-file")
	}

	return nil
}

func restoreNonValidator(from, configDir string) error {
	// Restore node_key.json
	srcNode := filepath.Join(from, "node_key.json")
	dstNode := filepath.Join(configDir, "node_key.json")
	if err := copyFileRestore(srcNode, dstNode); err != nil {
		return fmt.Errorf("restore node_key.json: %w", err)
	}
	uio.LogSuccess("Restored node_key.json")

	return nil
}

func copyFileRestore(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	return os.WriteFile(dst, data, 0o600)
}

func copyDirRestore(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		return copyFileRestore(path, dstPath)
	})
}
