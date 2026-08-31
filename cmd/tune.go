package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/role"
	"github.com/opscores/umesh-cli/internal/tune"
	"github.com/opscores/umesh-cli/internal/uio"
)

// newTuneCmd creates the tune command (setup phase).
func newTuneCmd() *cobra.Command {
	var roleOverride string
	cmd := &cobra.Command{
		Use:   "tune",
		Short: "Apply production tuning",
		Long: `Apply production tuning to config.toml and app.toml.

Useful for re-tuning without re-initialization (role change, parameter update).

Examples:
  umeshctl setup tune                      # auto-detect role
  umeshctl setup tune --role sentry        # override role`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedRole, err := role.Resolve(global.DataDir, roleOverride)
			if err != nil {
				return err
			}

			var r tune.Role
			switch resolvedRole {
			case "validator", "genesis":
				r = tune.RoleValidator
			case "sentry":
				r = tune.RoleSentry
			case "rpc":
				r = tune.RoleRPC
			default:
				return fmt.Errorf("invalid role %q: must be validator, sentry, or rpc", resolvedRole)
			}
			if err := tune.Apply(nodeinit.ConfigDir(), r); err != nil {
				return fmt.Errorf("tune failed: %w", err)
			}
			uio.LogSuccess("tuning applied for role=%s", resolvedRole)
			return nil
		},
	}
	cmd.Flags().StringVar(&roleOverride, "role", "", "Node role override (auto-detected if empty)")
	_ = cmd.RegisterFlagCompletionFunc("role", completeRoles())
	return cmd
}
