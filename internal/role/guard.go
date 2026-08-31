package role

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/nodeinit"
)

func Check(cmd *cobra.Command, allowed []string) (string, error) {
	var explicitRole string
	if f := cmd.Flags().Lookup("role"); f != nil {
		explicitRole = f.Value.String()
	}
	dataDir, _ := cmd.Flags().GetString("data-dir")
	if dataDir == "" {
		dataDir = nodeinit.DetectHome()
	}
	resolved, err := Resolve(dataDir, explicitRole)
	if err != nil {
		return "", err
	}
	for _, a := range allowed {
		if resolved == a {
			return resolved, nil
		}
	}
	return resolved, fmt.Errorf("command '%s' is not available for role '%s' (expected: %v)",
		cmd.CommandPath(), resolved, allowed)
}