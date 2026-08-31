package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newPruneCmd() *cobra.Command {
	var keepRecent int64
	var yes, dryRun bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune old blocks and application state",
		Long: `Manually trigger pruning of old blocks and application state.

Pruning frees disk space by removing old blocks and state data
that is no longer needed.

  umeshctl node prune                    # prune with current settings
  umeshctl node prune --keep-recent 1000 # keep last 1000 blocks
  umeshctl node prune --dry-run          # show what would be pruned`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun {
				uio.LogInfo("DRY RUN: would prune old blocks and application state")
				if keepRecent > 0 {
					uio.LogInfo("DRY RUN: would keep last %d blocks (custom pruning)", keepRecent)
				} else {
					uio.LogInfo("DRY RUN: would use current pruning settings")
				}
				return nil
			}

			uio.LogStep("Starting prune...")

			// Destructive: removes blocks/state — require explicit confirmation.
			ok, err := uio.Confirm("Prune old blocks and application state? This permanently removes historical data", yes)
			if err != nil {
				return err
			}
			if !ok {
				uio.LogInfo("Aborted.")
				return nil
			}

			docker := dkrcmd.New(
				dkrcmd.WithImage(global.Image),
				dkrcmd.WithHome(global.Home),
				dkrcmd.WithDataDir(global.DataDir),
				dkrcmd.WithBackupsDir(nodeinit.BackupsDir(global.DataDir)),
			)
			if docker.IsRunning() {
				uio.LogWarning("container %q is running — stop node first to avoid DB lock", global.Container)
			}

			// cosmos-sdk: `umeshnode prune [pruning-method] [--pruning-keep-recent N ...]`
			// (`umeshnode snapshots` has no `prune` subcommand; using it was a bug).
			pruneArgs := []string{"umeshnode", "prune"}
			if keepRecent > 0 {
				pruneArgs = append(pruneArgs, "custom",
					"--pruning-keep-recent", fmt.Sprint(keepRecent),
					"--app-db-backend", "goleveldb")
			}

			out, err := docker.RunMount(nil, pruneArgs...)
			if err != nil {
				return fmt.Errorf("prune failed: %w", err)
			}

			uio.LogSuccess("Prune completed")
			if len(out) > 0 {
				uio.Print(string(out))
			}
			return nil
		},
	}

	cmd.Flags().Int64Var(&keepRecent, "keep-recent", 0, "Number of recent blocks to keep")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be pruned without executing")
	return cmd
}
