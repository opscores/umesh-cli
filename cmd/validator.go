package cmd

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newValidatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validator",
		Short: "Validator operations",
		Long: `Validator lifecycle operations for Mainnet Ritual.

  umeshctl validator create               # create validator in network
  umeshctl validator check-balance        # early balance check (before sync)
  umeshctl validator generate-gentx       # generate gentx
  umeshctl validator operator-address     # get validator operator address
  umeshctl validator signing-info         # show signing info (missed blocks)
  umeshctl validator unjail               # recover after downtime jail
  umeshctl validator backup-consensus     # backup consensus keys`,
		Annotations: map[string]string{"role-guard": "validator"},
	}
	cmd.AddCommand(newValidatorCreateCmd())
	cmd.AddCommand(newValidatorCheckBalanceCmd())
	cmd.AddCommand(newValidatorGenerateGentxCmd())
	cmd.AddCommand(newValidatorOperatorAddressCmd())
	cmd.AddCommand(newValidatorSigningInfoCmd())
	cmd.AddCommand(newValidatorUnjailCmd())
	cmd.AddCommand(newValidatorBackupConsensusCmd())
	return cmd
}

const createValidatorGasLimit = 300_000

func newValidatorCreateCmd() *cobra.Command {
	var keyName, keyringPass, moniker, amount, commissionRate,
		commissionMaxRate, commissionMaxChangeRate, minSelfDelegation, from, chainID string
	var pubkey, gasPrices, denom, gasLimit string
	var gasAdjustment float64
	var yes bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create validator in network (post-genesis)",
		Long: `Register a new validator in an existing network.

Prerequisites: the node must be fully synced (not catching_up) and the operator
wallet must have sufficient balance for self-delegation plus gas fees.

Pubkey is auto-resolved from the running node via 'umeshd comet show-validator'.
Gas defaults (0.0025<denom>, 1.5 adjustment, auto gas) can be overridden.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if keyName == "" || from == "" {
				return fmt.Errorf("--key-name and --from are required")
			}
			if keyringPass == "" {
				return fmt.Errorf("--keyring-pass is required")
			}
			if moniker == "" {
				return fmt.Errorf("--moniker is required")
			}
			if amount == "" {
				return fmt.Errorf("--amount is required")
			}
			if chainID == "" {
				return fmt.Errorf("--chain-id is required")
			}

			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))

			// Default gas-prices from denom if not provided
			// (matches minimum-gas-prices in app.toml: 0.0025<denom>)
			if gasPrices == "" {
				gasPrices = "0.0025" + denom
			}

			// Pre-flight: container must be running (AFTER launch, online phase)
			if !docker.IsRunning() {
				return fmt.Errorf("container %q is not running — start node first: docker compose --profile validator up -d (or --container %s)", global.Container, global.Container)
			}

			// Pre-flight: check node is not catching_up
			if err := requireCaughtUp(docker); err != nil {
				return err
			}

			// Pre-flight: check operator has sufficient balance
			if err := requireSufficientBalance(docker, global.Home, from, amount, gasPrices); err != nil {
				return err
			}

			// Destructive/irreversible: require explicit confirmation on a TTY.
			ok, err := uio.Confirm(fmt.Sprintf("Register validator %q with %s self-delegation on chain %s? This sends a real transaction", moniker, amount, chainID), yes)
			if err != nil {
				return err
			}
			if !ok {
				uio.LogInfo("Aborted.")
				return nil
			}

			// Auto-resolve consensus pubkey from running node if not provided
			if pubkey == "" {
				out, err := docker.ExecOutput("umeshnode", "comet", "show-validator", "--home", global.Home)
				if err != nil {
					return fmt.Errorf("failed to get consensus pubkey: %w", err)
				}
				pubkey = strings.TrimSpace(out)
			}

			// Generate JSON config for create-validator (Cosmos SDK v0.54 compatible).
			// Host path is under DataDir (bind-mounted at global.Home inside container).
			// BEFORE launch: file would be written via RunMount; AFTER launch: host write
			// is visible inside running container via compose volume.
			hostJSONFile := filepath.Join(nodeinit.Home(), "config", "validator.json")
			containerJSONFile := filepath.Join(global.Home, "config", "validator.json")
			if err := generateValidatorJSON(hostJSONFile, []byte(pubkey), moniker, amount,
				commissionRate, commissionMaxRate, commissionMaxChangeRate,
				minSelfDelegation, denom); err != nil {
				return fmt.Errorf("generate validator json: %w", err)
			}
			// Clean up inside container after tx (host file is same inode via mount, but
			// remove via Exec to ensure correct permissions/path inside container).
			defer func() { _, _ = docker.ExecOutput("rm", "-f", containerJSONFile) }()

			// Build gas flags: either explicit --gas-limit or auto with adjustment
			var gasArgs []string
			if gasLimit != "" {
				gasArgs = []string{"--gas", gasLimit}
			} else {
				gasArgs = []string{"--gas", "auto", "--gas-adjustment", fmt.Sprintf("%.2f", gasAdjustment)}
			}

			args = []string{"umeshnode", "tx", "staking", "create-validator", containerJSONFile,
				"--from", from,
				"--chain-id", chainID,
				"--keyring-backend", "file",
				"--keyring-dir", global.Home + "/keyring",
				"--home", global.Home,
				"--yes",
				"--output", "json",
			}
			args = append(args, gasArgs...)
			args = append(args, "--gas-prices", gasPrices)

			// Password passed via stdin with trailing newline and docker exec -i (see dkrcmd.Docker.Exec).
			// Cosmos SDK reads password line-delimited; newline inside password is not supported.
			out, err := docker.Exec(strings.NewReader(keyringPass+"\n"), args...)
			if err != nil {
				return err
			}
			uio.LogSuccess("create-validator tx submitted: %s", strings.TrimSpace(string(out)))
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&keyName, "key-name", "", "Validator key name")
	f.StringVar(&keyringPass, "keyring-pass", "", "Keyring password (inline; prefer --keyring-password-file/-p at setup phase)")
	f.StringVar(&from, "from", "", "Delegator account address")
	f.StringVar(&moniker, "moniker", "", "Validator moniker")
	f.StringVar(&amount, "amount", "", "Stake amount+denom (e.g. 5000000uumesh)")
	f.StringVar(&commissionRate, "commission-rate", "0.10", "Commission rate")
	f.StringVar(&commissionMaxRate, "commission-max-rate", "0.20", "Max commission rate")
	f.StringVar(&commissionMaxChangeRate, "commission-max-change-rate", "0.01", "Max commission change rate")
	f.StringVar(&minSelfDelegation, "min-self-delegation", "1", "Minimum self delegation")
	f.StringVar(&chainID, "chain-id", "", "Chain ID")
	f.StringVar(&pubkey, "pubkey", "", "Validator consensus pubkey (auto-resolved if empty)")
	f.StringVar(&gasPrices, "gas-prices", "", "Gas prices (default: 0.0025<denom>)")
	f.Float64Var(&gasAdjustment, "gas-adjustment", 1.5, "Gas adjustment for --gas auto")
	f.StringVar(&denom, "denom", "uumesh", "Denom for gas-prices default")
	f.StringVar(&gasLimit, "gas-limit", "", "Gas limit (default: auto with 1.5 adjustment)")
	f.BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func newValidatorCheckBalanceCmd() *cobra.Command {
	var from, amount, gasPrices, denom string

	cmd := &cobra.Command{
		Use:   "check-balance",
		Short: "Check if funding is sufficient for create-validator (early, during sync)",
		Long: `Early check that the operator wallet has enough funds for self-delegation plus gas.

Runs before the node is fully synced — useful to request funding while the node syncs.

  umeshctl validator check-balance --from <umesh1...> --amount 5000000uumesh
  umeshctl validator check-balance --from <addr> --amount 5000000uumesh --denom uumesh --gas-prices 0.0025uumesh`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" {
				return fmt.Errorf("--from is required (operator address)")
			}
			if amount == "" {
				return fmt.Errorf("--amount is required (e.g. 5000000uumesh)")
			}
			if gasPrices == "" {
				gasPrices = "0.0025" + denom
			}
			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))
			if !docker.IsRunning() {
				return fmt.Errorf("container %q is not running — start node first: docker compose --env-file .env.validator --profile validator up -d", global.Container)
			}
			if err := requireSufficientBalance(docker, global.Home, from, amount, gasPrices); err != nil {
				return err
			}
			uio.LogSuccess("Funding OK for %s", from)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&from, "from", "", "Operator account address (umesh1...)")
	f.StringVar(&amount, "amount", "", "Stake amount+denom (e.g. 5000000uumesh)")
	f.StringVar(&gasPrices, "gas-prices", "", "Gas prices (default: 0.0025<denom>)")
	f.StringVar(&denom, "denom", "uumesh", "Denom for gas-prices default")
	return cmd
}

func newValidatorGenerateGentxCmd() *cobra.Command {
	var keyName, keyringPass, moniker, stakeAmount, denom, chainID,
		externalIP, outputDir string

	cmd := &cobra.Command{
		Use:   "generate-gentx",
		Short: "Generate gentx for Mainnet Ritual",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if keyName == "" {
				return fmt.Errorf("--key-name is required")
			}
			if keyringPass == "" {
				k, err := uio.ReadSecret("Keyring password: ")
				if err != nil {
					return err
				}
				keyringPass = k
			}

			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))
			gentxDir := global.Home + "/config/gentx"

			out, err := docker.Exec(strings.NewReader(keyringPass+"\n"),
				"umeshnode", "genesis", "gentx", keyName,
				stakeAmount+denom,
				"--chain-id", chainID,
				"--keyring-backend", "file",
				"--keyring-dir", global.Home+"/keyring",
				"--home", global.Home,
				"--moniker", moniker,
				"--output-document", gentxDir+"/mv-"+scrubbed(moniker)+"-gentx.json",
				"--yes")
			if err != nil {
				return err
			}
			uio.LogSuccess("gentx created: %s", strings.TrimSpace(string(out)))
			return copyGentxOut(docker, gentxDir, outputDir)
		},
	}

	f := cmd.Flags()
	f.StringVar(&keyName, "key-name", "validator", "Validator key name")
	f.StringVar(&keyringPass, "keyring-pass", "", "Keyring password (prompts if empty; prefer --keyring-password-file/-p at setup phase)")
	f.StringVar(&moniker, "moniker", "", "Validator moniker")
	f.StringVar(&stakeAmount, "stake-amount", "5000000", "Stake amount")
	f.StringVar(&denom, "denom", "uumesh", "Denom")
	f.StringVar(&chainID, "chain-id", "", "Chain ID")
	f.StringVar(&externalIP, "ip", "", "External IP")
	f.StringVar(&outputDir, "output", "./gentx", "Host directory for exports")
	return cmd
}

func newValidatorOperatorAddressCmd() *cobra.Command {
	var keyName, keyringPass, output string
	cmd := &cobra.Command{
		Use:   "operator-address",
		Short: "Get validator operator address",
		Long:  "Show the validator operator address (umeshvaloper1...) for a key.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if keyName == "" {
				keyName = "validator"
			}
			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))
			// Password passed via stdin with trailing newline (see dkrcmd.Docker.Exec).
			out, err := docker.Exec(strings.NewReader(keyringPass+"\n"), "umeshnode", "keys", "show", keyName,
				"--bech", "val",
				"--keyring-backend", "file",
				"--keyring-dir", global.Home+"/keyring",
				"--home", global.Home,
				"--output", "json")
			if err != nil {
				return fmt.Errorf("get operator address: %w", err)
			}
			var key struct {
				Address string `json:"address"`
			}
			addr := strings.TrimSpace(string(out))
			if json.Unmarshal(out, &key) == nil && key.Address != "" {
				addr = strings.TrimSpace(key.Address)
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			return uio.Emit(format, map[string]any{"operator_address": addr}, func() {
				uio.Print("%s", addr)
			})
		},
	}
	cmd.Flags().StringVar(&keyName, "key-name", "validator", "Validator key name")
	cmd.Flags().StringVar(&keyringPass, "keyring-pass", "", "Keyring password (inline; prefer --keyring-password-file/-p at setup phase)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newValidatorSigningInfoCmd() *cobra.Command {
	var consAddr, output string
	cmd := &cobra.Command{
		Use:   "signing-info",
		Short: "Show validator signing info",
		Long:  "Display signing info: missed blocks counter, tombstoned status, etc.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))
			out, err := docker.ExecOutput("umeshnode", "query", "slashing", "signing-info",
				consAddr,
				"--home", global.Home,
				"--output", "json")
			if err != nil {
				return fmt.Errorf("query signing-info: %w", err)
			}
			var info map[string]any
			if err := json.Unmarshal([]byte(out), &info); err != nil {
				info = map[string]any{"raw": strings.TrimSpace(out)}
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			return uio.Emit(format, info, func() {
				uio.Print("%s", strings.TrimSpace(out))
			})
		},
	}
	cmd.Flags().StringVar(&consAddr, "cons-addr", "", "Validator consensus address (valcons...)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newValidatorUnjailCmd() *cobra.Command {
	var keyName, keyringPass, chainID string
	cmd := &cobra.Command{
		Use:   "unjail",
		Short: "Unjail validator after downtime",
		Long:  "Submit unjail transaction to recover validator after downtime jail.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if keyName == "" {
				return fmt.Errorf("--key-name is required")
			}
			if keyringPass == "" {
				return fmt.Errorf("--keyring-pass is required")
			}
			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container))
			out, err := docker.Exec(strings.NewReader(keyringPass+"\n"),
				"umeshnode", "tx", "slashing", "unjail",
				"--from", keyName,
				"--chain-id", chainID,
				"--keyring-backend", "file",
				"--keyring-dir", global.Home+"/keyring",
				"--home", global.Home,
				"--yes",
				"--output", "json")
			if err != nil {
				return err
			}
			uio.LogSuccess("unjail tx submitted: %s", strings.TrimSpace(string(out)))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&keyName, "key-name", "", "Validator key name")
	f.StringVar(&keyringPass, "keyring-pass", "", "Keyring password (inline; prefer --keyring-password-file/-p at setup phase)")
	f.StringVar(&chainID, "chain-id", "", "Chain ID")
	return cmd
}

func newValidatorBackupConsensusCmd() *cobra.Command {
	var dataDir, outputDir string
	cmd := &cobra.Command{
		Use:   "backup-consensus",
		Short: "Backup consensus keys (priv_validator_key.json, node_key.json, priv_validator_state.json)",
		Long: `Backup consensus keys to a timestamped directory.

Backs up the following files from the node's config directory:
  - priv_validator_key.json (consensus secret key)
  - node_key.json (P2P identity key)
  - priv_validator_state.json (last signed height/round for double-sign protection)

Files are copied with chmod 600 permissions.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve data directory
			if dataDir == "" {
				dataDir = nodeinit.DetectHome()
			}
			backupsDir := nodeinit.BackupsDir(dataDir)

			// Create timestamped backup subdirectory
			ts := time.Now().Format("20060102-150405")
			backupDir := filepath.Join(backupsDir, "validator-consensus-"+ts)
			if err := os.MkdirAll(backupDir, 0o700); err != nil {
				return fmt.Errorf("create backup directory: %w", err)
			}

			files := []struct {
				name, sub string
			}{
				{"priv_validator_key.json", "config"},
				{"node_key.json", "config"},
				{"priv_validator_state.json", "data"},
			}

			for _, f := range files {
				src := filepath.Join(dataDir, f.sub, f.name)
				dst := filepath.Join(backupDir, f.name)
				data, err := os.ReadFile(src)
				if err != nil {
					if os.IsNotExist(err) {
						uio.LogWarning("File not found, skipping: %s", src)
						continue
					}
					return fmt.Errorf("read %s: %w", src, err)
				}
				if err := os.WriteFile(dst, data, 0o600); err != nil {
					return fmt.Errorf("write %s: %w", dst, err)
				}
				uio.LogInfo("Backed up %s -> %s", src, dst)
			}

			// Optional: also copy to explicit outputDir if provided
			if outputDir != "" {
				if err := os.MkdirAll(outputDir, 0o700); err != nil {
					return fmt.Errorf("create output directory: %w", err)
				}
				for _, f := range files {
					src := filepath.Join(dataDir, f.sub, f.name)
					dst := filepath.Join(outputDir, f.name)
					data, err := os.ReadFile(src)
					if err != nil {
						if os.IsNotExist(err) {
							continue
						}
						return fmt.Errorf("read %s: %w", src, err)
					}
					if err := os.WriteFile(dst, data, 0o600); err != nil {
						return fmt.Errorf("write %s: %w", dst, err)
					}
				}
				uio.LogSuccess("Consensus keys backed up to %s and %s", backupDir, outputDir)
			} else {
				uio.LogSuccess("Consensus keys backed up to %s", backupDir)
			}

			uio.LogWarning("Store backups offline! They contain consensus private keys.")
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&dataDir, "data-dir", "", "Node data directory (default: auto-detect)")
	f.StringVar(&outputDir, "output-dir", "", "Optional additional output directory")
	return cmd
}

// copyGentxOut copies the generated gentx out of the container to outputDir.
func copyGentxOut(docker *dkrcmd.Docker, gentxDir, outputDir string) error {
	out, err := docker.ExecOutput("sh", "-c", "ls "+gentxDir)
	if err != nil {
		return fmt.Errorf("list gentx dir: %w", err)
	}
	name := strings.TrimSpace(out)
	if name == "" {
		return fmt.Errorf("no gentx found in %s", gentxDir)
	}
	content, err := docker.ExecOutput("cat", gentxDir+"/"+name)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return fmt.Errorf("invalid gentx json: %w", err)
	}
	return writeHostFile(outputDir+"/"+name, []byte(content))
}

func scrubbed(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func writeHostFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// requireCaughtUp verifies the node is fully synchronized before proceeding.
// Online phase: requires running container (docker exec). Returns an error if
// the container is not running, still catching up, or has no blocks.
func requireCaughtUp(d *dkrcmd.Docker) error {
	if !d.IsRunning() {
		return fmt.Errorf("container %q is not running — start node first", d.Container)
	}
	out, err := d.ExecOutput("umeshnode", "status", "--home", global.Home, "--output", "json")
	if err != nil {
		return fmt.Errorf("status check: %w", err)
	}

	var status struct {
		SyncInfo struct {
			CatchingUp        bool   `json:"catching_up"`
			LatestBlockHeight string `json:"latest_block_height"`
		} `json:"SyncInfo"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		return fmt.Errorf("parse status output: %w", err)
	}

	height, _ := strconv.ParseInt(status.SyncInfo.LatestBlockHeight, 10, 64)
	if status.SyncInfo.CatchingUp || height == 0 {
		return fmt.Errorf("node is not fully synced (catching_up=%v, height=%d); wait for full sync before creating validator: umeshctl node health --wait-sync", status.SyncInfo.CatchingUp, height)
	}
	return nil
}

// requireSufficientBalance checks that the operator has enough funds for self-delegation plus estimated fees.
func requireSufficientBalance(d *dkrcmd.Docker, home, from, amount, gasPrices string) error {
	if len(amount) == 0 {
		return fmt.Errorf("amount is required")
	}

	// Extract denom and numeric part from amount (e.g., "5000000uumesh" -> amount="5000000", denom="uumesh")
	delim := strings.IndexFunc(amount, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if delim < 0 {
		return fmt.Errorf("could not parse amount %q", amount)
	}
	amountStr := amount[:delim]
	amountDenom := amount[delim:]

	out, err := d.ExecOutput("umeshnode", "query", "bank", "balances", from,
		"--home", home, "--output", "json")
	if err != nil {
		return fmt.Errorf("query balance: %w", err)
	}

	var balRes struct {
		Balances []struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"balances"`
	}
	if err := json.Unmarshal([]byte(out), &balRes); err != nil {
		return fmt.Errorf("parse balance: %w", err)
	}

	balanceStr := "0"
	for _, b := range balRes.Balances {
		if b.Denom == amountDenom {
			balanceStr = b.Amount
			break
		}
	}

	balanceInt, ok := new(big.Int).SetString(balanceStr, 10)
	if !ok {
		return fmt.Errorf("invalid balance amount: %s", balanceStr)
	}
	delAmtInt, ok := new(big.Int).SetString(amountStr, 10)
	if !ok {
		return fmt.Errorf("invalid delegation amount: %s", amountStr)
	}

	// Estimate gas fee: gas_prices * gas_limit
	// Extract numeric part of gasPrices (e.g., "0.0025uumesh" -> 0.0025)
	gasPriceNum := strings.TrimSuffix(gasPrices, amountDenom)
	gasPriceRat, ok := new(big.Rat).SetString(gasPriceNum)
	if !ok {
		return fmt.Errorf("could not parse gas price: %s", gasPrices)
	}

	gasLimit := new(big.Int).SetInt64(createValidatorGasLimit)
	estimatedFeeRat := new(big.Rat).Mul(gasPriceRat, new(big.Rat).SetInt(gasLimit))

	// Convert fee to integer (floor)
	estimatedFeeInt, _ := estimatedFeeRat.Float64()
	estimatedFee := big.NewInt(int64(estimatedFeeInt))

	// Total required = amount + estimated fee
	totalRequired := new(big.Int).Add(delAmtInt, estimatedFee)

	if balanceInt.Cmp(totalRequired) < 0 {
		return fmt.Errorf("insufficient balance: account %s has %s %s, need %s (%s delegation + %s fee)",
			from, balanceStr, amountDenom, totalRequired.String(), amountStr, estimatedFee.String())
	}
	uio.LogInfo("Balance OK: account %s has %s %s (delegation: %s, est fee: %s)", from, balanceStr, amountDenom, amountStr, estimatedFee.String())
	return nil
}

// generateValidatorJSON creates a JSON config file for create-validator command.
// Compatible with Cosmos SDK v0.54 which requires JSON input instead of CLI flags.
// The pubkey parameter should be the raw JSON output from `comet show-validator`
// (e.g. {"@type":"/cosmos.crypto.ed25519.PubKey","key":"..."}).
func generateValidatorJSON(outputPath string, pubkeyJSON []byte, moniker, amount,
	commissionRate, commissionMaxRate, commissionMaxChangeRate,
	minSelfDelegation, denom string) error {
	// pubkeyJSON is the raw JSON from `comet show-validator` — parse and re-emit
	// as an Any-compatible structure for the create-validator config.
	validatorCfg := struct {
		Pubkey            map[string]interface{} `json:"pubkey"`
		Amount            string                 `json:"amount"`
		Moniker           string                 `json:"moniker"`
		Identity          string                 `json:"identity"`
		Website           string                 `json:"website"`
		SecurityContact   string                 `json:"security-contact"`
		Details           string                 `json:"details"`
		Commission        map[string]string      `json:"commission"`
		MinSelfDelegation string                 `json:"min-self-delegation"`
	}{
		Amount:  amount,
		Moniker: moniker,
		Commission: map[string]string{
			"rate":            commissionRate,
			"max-rate":        commissionMaxRate,
			"max-change-rate": commissionMaxChangeRate,
		},
		MinSelfDelegation: minSelfDelegation,
	}

	// Parse the pubkey JSON to extract @type and key fields
	var pubkey struct {
		Type string `json:"@type"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(pubkeyJSON, &pubkey); err != nil {
		return fmt.Errorf("parse pubkey json: %w", err)
	}

	validatorCfg.Pubkey = map[string]interface{}{
		"@type": pubkey.Type,
		"key":   pubkey.Key,
	}

	data, err := json.MarshalIndent(validatorCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal validator json: %w", err)
	}

	return os.WriteFile(outputPath, data, 0o600)
}
