package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newLogsCmd() *cobra.Command {
	var level, module, since string
	var tail int
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View and filter node logs",
		Long: `View logs from the node container with optional filtering.

Uses docker logs on the container selected via --container (default
umesh-validator). Filtering is done locally using grep on the log output.

  umeshctl node logs                              # all logs
  umeshctl node logs --level error                # only errors
  umeshctl node logs --module consensus           # only consensus module
  umeshctl node logs --since 1h                   # last hour
  umeshctl node logs --tail 100                   # last 100 lines
  umeshctl node logs -f                           # follow`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Build docker logs command for the container selected via --container.
			args2 := []string{"logs"}
			if follow {
				args2 = append(args2, "-f")
			}
			if tail > 0 {
				args2 = append(args2, "--tail", fmt.Sprintf("%d", tail))
			}
			if since != "" {
				args2 = append(args2, "--since", since)
			}

			// If no filtering, just run docker logs directly
			if level == "" && module == "" {
				return runDockerLogs(append(args2, global.Container))
			}

			// With filtering: get logs, then filter with grep
			args2 = append(args2, global.Container)
			output, err := getDockerLogs(args2)
			if err != nil {
				return err
			}

			// Apply filters
			lines := strings.Split(output, "\n")
			var filtered []string
			for _, line := range lines {
				if matchesFilter(line, level, module) {
					filtered = append(filtered, line)
				}
			}

			if len(filtered) == 0 {
				uio.LogInfo("No log lines match the filter")
				return nil
			}

			for _, line := range filtered {
				uio.Print("%s", line)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&level, "level", "", "Filter by log level (error, warn, info, debug)")
	cmd.Flags().StringVar(&module, "module", "", "Filter by module name")
	cmd.Flags().StringVar(&since, "since", "", "Show logs since (e.g. 1h, 30m)")
	cmd.Flags().IntVar(&tail, "tail", 0, "Number of lines to show (0 = all)")
	cmd.Flags().BoolVar(&follow, "follow", false, "Follow log output")

	return cmd
}

func runDockerLogs(args []string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func getDockerLogs(args []string) (string, error) {
	cmd := exec.Command("docker", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker logs: %w", err)
	}
	return string(output), nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func matchesFilter(line, level, module string) bool {
	if level != "" {
		// Support key=value style ("level=info") and JSON style ("level":"info")
		if !strings.Contains(line, "level="+level) && !strings.Contains(line, "level="+strings.ToUpper(level)) &&
			!strings.Contains(line, `"level":"`+level+`"`) && !strings.Contains(line, `"level":"`+strings.ToUpper(level)+`"`) {
			// Also check for direct level mentions
			if !strings.Contains(line, "["+level+"]") && !strings.Contains(line, "["+strings.ToUpper(level)+"]") {
				return false
			}
		}
	}
	if module != "" {
		// Support key=value style ("module=state") and JSON style ("module":"state")
		if !strings.Contains(line, "module="+module) && !strings.Contains(line, "module="+capitalize(module)) &&
			!strings.Contains(line, `"module":"`+module+`"`) && !strings.Contains(line, `"module":"`+capitalize(module)+`"`) {
			return false
		}
	}
	return true
}
