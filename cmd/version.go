package cmd

import (
	"runtime"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/uio"
)

var (
	// Version is set at build time via -ldflags.
	Version = "dev"
	// GitCommit is set at build time via -ldflags.
	GitCommit = "unknown"
	// BuildDate is set at build time via -ldflags.
	BuildDate = "unknown"
)

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  "Display the version of umeshctl and build details.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			short, _ := cmd.Flags().GetBool("short")
			if short {
				uio.Print(Version)
				return nil
			}
			uio.Print("umeshctl version: %s", Version)
			uio.Print("git commit:      %s", GitCommit)
			uio.Print("build date:      %s", BuildDate)
			uio.Print("go version:      %s", runtime.Version())
			uio.Print("os/arch:         %s/%s", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
	cmd.Flags().Bool("short", false, "Print only the version number")
	return cmd
}
