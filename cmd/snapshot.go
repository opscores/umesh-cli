package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newSnapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Manage state sync snapshots",
		Long: `Create, list, and restore state sync snapshots.

  umeshctl node snapshot create --output ./snapshots # create snapshot
  umeshctl node snapshot list                         # list snapshots
  umeshctl node snapshot restore --from ./snapshots   # restore snapshot`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newSnapshotCreateCmd())
	cmd.AddCommand(newSnapshotListCmd())
	cmd.AddCommand(newSnapshotRestoreCmd())
	return cmd
}

func newSnapshotCreateCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Export a state-sync snapshot to a portable archive",
		Long: `Export the latest local state-sync snapshot to a portable archive.

State-sync snapshots are created automatically by the node every
snapshot-interval blocks (configure [state-sync].snapshot-interval in app.toml).
This command lists the local snapshots, picks the newest, and exports it with
"umeshnode snapshots dump" into <output>.

  umeshctl node snapshot create --output ./snapshots`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				output = "./snapshots"
			}

			uio.LogStep("Creating snapshot...")

			docker := dkrcmd.New(
				dkrcmd.WithImage(global.Image),
				dkrcmd.WithHome(global.Home),
				dkrcmd.WithDataDir(global.DataDir),
				dkrcmd.WithBackupsDir(nodeinit.BackupsDir(global.DataDir)),
				// Bind the host output dir into the throwaway container so the
				// dumped archive lands on the host (mirrors the restore flow).
				dkrcmd.WithExtraMount(output, "/snapshot-out"),
			)
			// Offline phase: RunMount uses a bind-mount — warn if the node is
			// live to avoid a DB lock.
			if docker.IsRunning() {
				uio.LogWarning("container %q is running — stop node first to avoid DB lock", global.Container)
			}

			// Discover the newest snapshot: `umeshnode snapshots list` prints
			// "height: N format: F chunks: C" per entry. (`umeshnode snapshots`
			// has no `create` subcommand; dump is the export primitive.)
			listOut, err := docker.RunMount(nil, "umeshnode", "snapshots", "list", "--home", global.Home)
			if err != nil {
				return fmt.Errorf("snapshot list failed: %w", err)
			}
			height, format, ok := latestSnapshot(listOut)
			if !ok {
				uio.LogWarning("no state-sync snapshots found — set [state-sync].snapshot-interval > 0 in app.toml and run the node to generate one")
				return nil
			}

			file := fmt.Sprintf("snapshot-%d-%d.snap", height, format)
			out, err := docker.RunMount(nil, "umeshnode", "snapshots", "dump",
				fmt.Sprint(height), fmt.Sprint(format),
				"-o", "/snapshot-out/"+file, "--home", global.Home)
			if err != nil {
				return fmt.Errorf("snapshot dump failed: %w", err)
			}

			uio.LogSuccess("Snapshot created: %s/%s", output, file)
			if len(out) > 0 {
				uio.Print(string(out))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&output, "output", "./snapshots", "Output directory for snapshot archive")
	return cmd
}

// latestSnapshot parses the plain-text output of `umeshnode snapshots list`
// (one "height: N format: F chunks: C" line per snapshot) and returns the
// highest height together with its archive format.
func latestSnapshot(out []byte) (height, format int64, ok bool) {
	re := regexp.MustCompile(`height:\s*(\d+)\s+format:\s*(\d+)`)
	var max int64 = -1
	for _, line := range strings.Split(string(out), "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		h, _ := strconv.ParseInt(m[1], 10, 64)
		f, _ := strconv.ParseInt(m[2], 10, 64)
		if h > max {
			max = h
			height, format, ok = h, f, true
		}
	}
	return
}

// parseSnapshotName extracts (height, format) from a portable archive named
// like "snapshot-1595-3.snap" (as produced by `umeshnode snapshots dump`).
func parseSnapshotName(name string) (height, format int64, ok bool) {
	m := regexp.MustCompile(`snapshot-(\d+)-(\d+)\.snap$`).FindStringSubmatch(name)
	if m == nil {
		return 0, 0, false
	}
	height, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	format, err = strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return height, format, true
}

func newSnapshotListCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available snapshots",
		Long:  "List snapshots available on the node for state sync.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			docker := dkrcmd.New(
				dkrcmd.WithImage(global.Image),
				dkrcmd.WithHome(global.Home),
				dkrcmd.WithDataDir(global.DataDir),
				dkrcmd.WithBackupsDir(nodeinit.BackupsDir(global.DataDir)),
			)
			if docker.IsRunning() {
				uio.LogWarning("container %q is running — stop node first to avoid DB lock", global.Container)
			}

			out, err := docker.RunMount(nil, "umeshnode", "snapshots", "list", "--home", global.Home)
			if err != nil {
				return fmt.Errorf("snapshot list failed: %w", err)
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}

			snapshots := parseSnapshotList(out)
			return uio.Emit(format, snapshots, func() {
				for _, s := range snapshots {
					uio.Print("Height: %d  Format: %d  Chunks: %d", s["height"], s["format"], s["chunks"])
				}
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

// parseSnapshotList parses the plain-text output of `umeshnode snapshots list`
// into a slice of maps for structured output.
func parseSnapshotList(out []byte) []map[string]any {
	re := regexp.MustCompile(`height:\s*(\d+)\s+format:\s*(\d+)\s+chunks:\s*(\d+)`)
	var snapshots []map[string]any
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		h, _ := strconv.ParseInt(m[1], 10, 64)
		f, _ := strconv.ParseInt(m[2], 10, 64)
		c, _ := strconv.ParseInt(m[3], 10, 64)
		snapshots = append(snapshots, map[string]any{
			"height":  h,
			"format":  f,
			"chunks":  c,
		})
	}
	return snapshots
}

func newSnapshotRestoreCmd() *cobra.Command {
	var from string
	var yes bool

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore from a snapshot",
		Long: `Restore the node state from a previously created snapshot.

WARNING: This will replace the current node data. Make sure the node is stopped.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" {
				return fmt.Errorf("--from is required")
			}

			ok, err := uio.Confirm("This will replace current node data. Ensure the node is stopped. Proceed?", yes)
			if err != nil {
				return err
			}
			if !ok {
				uio.LogInfo("Aborted.")
				return nil
			}

			// cosmos `snapshots restore` restores from the LOCAL snapshot store
			// by <height> <format> (it does not take an archive path), so the
			// portable archive must be loaded first.
			info, err := os.Stat(from)
			if err != nil {
				return fmt.Errorf("snapshot path not found: %s: %w", from, err)
			}

			// --from may be a directory holding a *.snap archive, or the archive
			// file itself. Resolve the archive name + the host dir to mount.
			var archive, mountParent string
			if info.IsDir() {
				mountParent = from
				entries, err := os.ReadDir(from)
				if err != nil {
					return fmt.Errorf("read snapshot dir: %w", err)
				}
				for _, e := range entries {
					if strings.HasSuffix(e.Name(), ".snap") {
						archive = e.Name()
						break
					}
				}
				if archive == "" {
					return fmt.Errorf("no *.snap archive found in %s", from)
				}
			} else {
				archive = filepath.Base(from)
				mountParent = filepath.Dir(from)
			}

			height, format, ok := parseSnapshotName(archive)
			if !ok {
				return fmt.Errorf("cannot parse height/format from %s (expected snapshot-<height>-<format>.snap)", archive)
			}

			uio.LogStep("Restoring snapshot height=%d format=%d from %s", height, format, archive)

			docker := dkrcmd.New(
				dkrcmd.WithImage(global.Image),
				dkrcmd.WithHome(global.Home),
				dkrcmd.WithDataDir(global.DataDir),
				dkrcmd.WithBackupsDir(nodeinit.BackupsDir(global.DataDir)),
				// Bind the host --from dir at /restore so the archive is visible.
				dkrcmd.WithExtraMount(mountParent, "/restore"),
			)
			if docker.IsRunning() {
				uio.LogWarning("container %q is running — stop node first; restore will replace live data and may cause corruption", global.Container)
			}

			if _, err := docker.RunMount(nil, "umeshnode", "snapshots", "load",
				"/restore/"+archive, "--home", global.Home); err != nil {
				return fmt.Errorf("snapshot load failed: %w", err)
			}
			out, err := docker.RunMount(nil, "umeshnode", "snapshots", "restore",
				fmt.Sprint(height), fmt.Sprint(format), "--home", global.Home)
			if err != nil {
				return fmt.Errorf("snapshot restore failed: %w", err)
			}

			uio.LogSuccess("Snapshot restored (height=%d, format=%d)", height, format)
			if len(out) > 0 {
				uio.Print(string(out))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Path to snapshot directory")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}
