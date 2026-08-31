package cmd

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/backup"
	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/role"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newBackupCmd() *cobra.Command {
	var roleOverride string
	var output string
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup node keys and configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedRole, err := role.Resolve(global.DataDir, roleOverride)
			if err != nil {
				return err
			}
			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))
			dir, err := backup.Run(docker, resolvedRole, output)
			if err != nil {
				// Fatal logs a colored [ERROR] and exits, so main.go does not
				// re-print the same error (avoids double output).
				uio.Fatal(err)
			}
uio.LogSuccess("Backup complete: %s", dir)
	// keyring/ (account keys) is intentionally NOT backed up here —
	uio.LogWarning("keyring/ is NOT included — back it up separately:")
	uio.LogWarning("  cp -r %s/keyring <safe-location>", hostKeyringPath(global.DataDir))
	// Keyring password file (if using --auto-password) is stored in XDG config dir
	uio.LogWarning("Keyring password file (if any) is at: %s/keyring.pass", nodeinit.KeyringConfigDir())
	return nil
		},
	}
	cmd.Flags().StringVar(&roleOverride, "role", "", "Node role override (auto-detected if empty)")
	cmd.Flags().StringVar(&output, "output", "./backups", "Backup output directory")
	_ = cmd.RegisterFlagCompletionFunc("role", completeRoles())
	return cmd
}

// backupKeyring is NOT copied by backup.Run: only consensus/node/genesis files
// are backed up. Warn the operator loudly on a successful backup so a validator
// does not discover the omission during a disaster restore.
//
// (README §11 documents this; this warning makes it visible from the CLI itself.)

// hostKeyringPath returns the host-side path to the keyring directory for the
// given data dir (or the role-default data dir when dataDir is empty).
// Inside the container the keyring lives at <home>/keyring, which is bind-mounted
// from <dataDir>/keyring on the host (see dkrcmd docker.go).
func hostKeyringPath(dataDir string) string {
	base := dataDir
	if base == "" {
		base = nodeinit.DetectHome()
	}
	return filepath.Join(base, "keyring")
}
