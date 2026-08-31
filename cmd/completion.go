package cmd

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
)

// validRoles are the accepted values for init <role> and --role flags.
var validRoles = []string{"genesis", "validator", "sentry", "rpc"}

// pruneStrategies are the accepted values for --pruning / node.pruning.
var pruneStrategies = []string{"custom", "everything", "default", "nothing"}

// doctorChecks are the accepted values for ops doctor --check.
var doctorChecks = []string{"arch", "ntp", "gitignore", "readiness", "wasmvm", "container-health", "peers", "p2p", "join"}

// outputFormats are the accepted values for --output.
var outputFormats = []string{"table", "json", "yaml"}

// completeOutputFormats returns a cobra completion function suggesting --output values.
func completeOutputFormats() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return outputFormats, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeRoles returns a cobra completion function suggesting valid roles.
func completeRoles() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return validRoles, cobra.ShellCompDirectiveNoFileComp
	}
}

// completePrune returns a cobra completion function suggesting pruning strategies.
func completePrune() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return pruneStrategies, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeDoctorChecks returns a cobra completion function suggesting --check values.
func completeDoctorChecks() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return doctorChecks, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeYAMLFiles completes paths of .yaml and .yml files (for --config).
func completeYAMLFiles() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
	}
}

// completeKeyNames returns a cobra completion function that lists keyring keys
// via `umeshd keys list` inside the container. Best-effort: any failure yields
// no completions rather than breaking the shell.
func completeKeyNames() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		names, err := listKeyringNames()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}
}

// listKeyringNames executes `umeshd keys list` in the container and extracts
// key names from the JSON output. Returns an empty slice on any error.
func listKeyringNames() ([]string, error) {
	out, err := execKeysRaw(nil, "list", "--output", "json")
	if err != nil {
		return nil, err
	}
	var keys []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &keys); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		if strings.TrimSpace(k.Name) != "" {
			names = append(names, k.Name)
		}
	}
	return names, nil
}
