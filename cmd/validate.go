package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/uio"
	"github.com/opscores/umesh-cli/internal/yamlconfig"
)

func newValidateConfigCmd() *cobra.Command {
	var configFile string
	var output string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a node configuration YAML file",
		Long: `Validate a node configuration YAML file.

Checks structure, required fields, role-specific constraints,
and that no secrets are stored in the config.

Example:
  umeshctl setup validate --config config/nodes/validator-genesis.yaml
  umeshctl setup validate --config config/nodes/validator-genesis.yaml --output json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if configFile == "" {
				return fmt.Errorf("--config is required")
			}

			cfg, err := yamlconfig.LoadYAML(configFile)
			if err != nil {
				return err
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}

			result := map[string]any{
				"valid":      true,
				"role":       cfg.Role,
				"chain_id":   cfg.Chain.ChainID,
				"moniker":    cfg.Node.Moniker,
				"denom":      cfg.Chain.Denom,
			}
			if cfg.Validator != nil {
				result["validator_key_name"] = cfg.Validator.KeyName
			}
			if cfg.Telemetry != nil && cfg.Telemetry.Endpoint != "" {
				result["telemetry"] = map[string]string{"enabled": "true", "endpoint": cfg.Telemetry.Endpoint}
			} else {
				result["telemetry"] = map[string]string{"enabled": "false"}
			}

			return uio.Emit(format, result, func() {
				uio.LogSuccess("Config file %s is valid", configFile)
				uio.Print("  role:     %s", cfg.Role)
				uio.Print("  chainId:  %s", cfg.Chain.ChainID)
				uio.Print("  moniker:  %s", cfg.Node.Moniker)
				uio.Print("  denom:    %s", cfg.Chain.Denom)

				if cfg.Validator != nil {
					uio.Print("  validator keyName: %s", cfg.Validator.KeyName)
				}
				if cfg.Telemetry != nil && cfg.Telemetry.Endpoint != "" {
					uio.LogSuccess("  telemetry: enabled (%s)", cfg.Telemetry.Endpoint)
				} else {
					uio.Print("  telemetry: off")
				}
			})
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "", "Path to node configuration YAML file")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	_ = cmd.RegisterFlagCompletionFunc("config", completeYAMLFiles())
	return cmd
}
