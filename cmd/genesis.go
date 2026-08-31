package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/uio"
)

func newGenesisCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "genesis",
		Short: "Genesis document utilities",
		Long: `Genesis document utilities for coordinators.

Fetch, validate, inspect, and patch the genesis document.
Collect gentx from validators for Mainnet Ritual.
Execute production genesis plans.

  umeshctl genesis fetch --url <url>      # fetch genesis document
  umeshctl genesis inspect                # show genesis info
  umeshctl genesis validate               # validate genesis
  umeshctl genesis collect-gentx --repo <url>  # collect gentx
  umeshctl genesis plan --config plan.yaml      # production genesis
  umeshctl genesis add-account ...            # incremental account
  umeshctl genesis add-validator ...          # incremental validator
  umeshctl genesis validate-plan ...          # validate plan`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newGenesisFetchCmd())
	cmd.AddCommand(newGenesisInspectCmd())
	cmd.AddCommand(newGenesisValidateCmd())
	cmd.AddCommand(newGenesisSetParamCmd())
	cmd.AddCommand(newGenesisSetTimeCmd())
	cmd.AddCommand(newGenesisCollectGentxCmd())
	cmd.AddCommand(newGenesisPlanCmd())
	cmd.AddCommand(newGenesisReportCmd())
	cmd.AddCommand(newGenesisValidatePlanCmd())
	cmd.AddCommand(newGenesisAddAccountCmd())
	cmd.AddCommand(newGenesisAddValidatorCmd())
	return cmd
}

func newGenesisFetchCmd() *cobra.Command {
	var params nodeinit.FetchGenesisParams
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch genesis document from URL",
		Long: `Download a genesis document from the given URL and save it
to the node config directory.

The operator explicitly specifies the source URL. No fallback chains
or automatic source selection.

If --denom is provided, it will be validated against the genesis bond_denom.
If not provided, the denom will be auto-extracted from genesis.

  umeshctl genesis fetch --url https://example.com/genesis.json
  umeshctl genesis fetch --url http://10.0.0.5:26657/genesis --sha256 <hash>
  umeshctl genesis fetch --url http://10.0.0.6:26657/genesis --chain-id umesh-1
  umeshctl genesis fetch --url http://10.0.0.5:26657/genesis --dry-run   # validate without writing`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.URL == "" {
				return fmt.Errorf("--url is required")
			}
			params.WriteFile = !dryRun
			res, err := nodeinit.FetchGenesis(params)
			if err != nil {
				return err
			}
			if dryRun {
				uio.LogSuccess("Genesis dry-run OK: chain-id=%s, bond_denom=%s (not written)", res.ChainID, res.BondDenom)
			} else {
				uio.LogSuccess("Genesis fetched: chain-id=%s, bond_denom=%s", res.ChainID, res.BondDenom)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&params.URL, "url", "", "URL to fetch genesis from (required)")
	cmd.Flags().StringVar(&params.SHA256, "sha256", "", "Expected SHA256 hash for validation")
	cmd.Flags().StringVar(&params.ChainID, "chain-id", "", "Expected chain-ID for validation")
	cmd.Flags().StringVar(&params.Denom, "denom", "", "Expected bond denom for validation (auto-extracted if empty)")
	cmd.Flags().StringVar(&params.Output, "output", "", "Output file path (default: <home>/config/genesis.json)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate genesis without writing to disk")

	_ = cmd.MarkFlagRequired("url")

	return cmd
}

func newGenesisInspectCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Show genesis document info",
		Long: `Display key information from the local genesis document:
chain_id, denom, validators, total supply.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			genesisPath := nodeinit.GenesisFile()
			data, err := os.ReadFile(genesisPath)
			if err != nil {
				return fmt.Errorf("read genesis: %w", err)
			}

			var doc struct {
				ChainID  string `json:"chain_id"`
				AppState struct {
					Staking struct {
						Params struct {
							BondDenom string `json:"bond_denom"`
						} `json:"params"`
					} `json:"staking"`
					Bank struct {
						Balances []struct {
							Address string `json:"address"`
							Coins   []struct {
								Denom  string `json:"denom"`
								Amount string `json:"amount"`
							} `json:"coins"`
						} `json:"balances"`
					} `json:"bank"`
				} `json:"app_state"`
			}
			if err := json.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("parse genesis: %w", err)
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			return uio.Emit(format, map[string]any{
				"chain_id": doc.ChainID,
				"denom":    doc.AppState.Staking.Params.BondDenom,
				"accounts": len(doc.AppState.Bank.Balances),
			}, func() {
				uio.Print("Chain ID:    %s", doc.ChainID)
				uio.Print("Denom:       %s", doc.AppState.Staking.Params.BondDenom)
				uio.Print("Accounts:    %d", len(doc.AppState.Bank.Balances))
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newGenesisValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate genesis document",
		Long:  `Run umeshd genesis validate-genesis against the local genesis file.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			docker := dkrcmd.New(
				dkrcmd.WithImage(global.Image),
				dkrcmd.WithHome(global.Home),
				dkrcmd.WithDataDir(global.DataDir),
				dkrcmd.WithBackupsDir(nodeinit.BackupsDir(global.DataDir)),
			)
			if _, err := docker.RunMount(nil, "umeshnode", "genesis", "validate-genesis", "--home", global.Home); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
			uio.LogSuccess("genesis is valid")
			return nil
		},
	}
}

func newGenesisSetParamCmd() *cobra.Command {
	var path, value string
	cmd := &cobra.Command{
		Use:   "set-param",
		Short: "Set a parameter in genesis.json",
		Long: `Patch a parameter in the local genesis document.
Path uses dot notation (e.g. app_state.staking.params.max_validators).
Value is parsed as JSON: numbers, strings, booleans, objects.

Examples:
  umeshctl genesis set-param --path app_state.staking.params.max_validators --value 100
  umeshctl genesis set-param --path app_state.gov.params.voting_period --value 604800s`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if path == "" {
				return fmt.Errorf("--path is required")
			}
			if value == "" {
				return fmt.Errorf("--value is required")
			}
			if err := nodeinit.PatchGenesisParam(path, value); err != nil {
				return err
			}
			uio.LogSuccess("set %s = %s", path, value)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "dot-separated path (e.g. app_state.staking.params.max_validators)")
	cmd.Flags().StringVar(&value, "value", "", "value to set (JSON format)")
	_ = cmd.MarkFlagRequired("path")
	_ = cmd.MarkFlagRequired("value")
	return cmd
}

func newGenesisSetTimeCmd() *cobra.Command {
	var timeStr string
	cmd := &cobra.Command{
		Use:   "set-time",
		Short: "Set genesis_time in genesis.json",
		Long: `Set the genesis_time field in the local genesis document.
Used for Mainnet Ritual to set the chain launch time.

Example:
  umeshctl genesis set-time --time 2025-01-01T00:00:00Z`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeStr == "" {
				return fmt.Errorf("--time is required")
			}
			if err := nodeinit.SetGenesisTime(timeStr); err != nil {
				return err
			}
			uio.LogSuccess("set genesis_time = %s", timeStr)
			return nil
		},
	}
	cmd.Flags().StringVar(&timeStr, "time", "", "RFC3339 timestamp (e.g. 2025-01-01T00:00:00Z)")
	_ = cmd.MarkFlagRequired("time")
	return cmd
}

func newGenesisCollectGentxCmd() *cobra.Command {
	var repoURL, dataDir, chainID string

	cmd := &cobra.Command{
		Use:   "collect-gentx",
		Short: "Download gentx files from a GitHub repo and collect into genesis",
		Long: `Download a ZIP archive containing gentx JSON files from a GitHub repository,
extract them into the node's gentx directory, and run 'umeshd genesis collect-gentxs'.

This supports the Mainnet Ritual flow where validators submit gentx via Pull Request
to a centralized repository.`,
		Example: `  umeshctl genesis collect-gentx --repo https://github.com/org/gentx-repo/archive/refs/heads/main.zip --chain-id umesh-1
  umeshctl genesis collect-gentx --repo https://github.com/org/gentx-repo/releases/download/v1.0.0/gentx.zip --chain-id umesh-1`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoURL == "" {
				return fmt.Errorf("--repo is required (URL to ZIP archive with gentx files)")
			}
			if dataDir == "" {
				dataDir = nodeinit.DetectHome()
			}

			targetGentxDir := filepath.Join(dataDir, "config", "gentx")
			tmpDir, err := os.MkdirTemp("", "umeshctl-gentx-*")
			if err != nil {
				return fmt.Errorf("create temp dir: %w", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			zipPath := filepath.Join(tmpDir, "gentx.zip")

			uio.LogStep("Downloading gentx archive from %s", repoURL)
			if err := downloadGentxFile(repoURL, zipPath); err != nil {
				return fmt.Errorf("download gentx archive: %w", err)
			}
			uio.LogSuccess("Downloaded to %s", zipPath)

			uio.LogStep("Extracting gentx files...")
			extracted, err := extractGentxFiles(zipPath, tmpDir)
			if err != nil {
				return fmt.Errorf("extract gentx archive: %w", err)
			}
			if len(extracted) == 0 {
				return fmt.Errorf("no gentx JSON files found in archive")
			}
			uio.LogSuccess("Found %d gentx file(s)", len(extracted))

			if err := os.MkdirAll(targetGentxDir, 0o755); err != nil {
				return fmt.Errorf("create gentx dir: %w", err)
			}

			for _, src := range extracted {
				dst := filepath.Join(targetGentxDir, filepath.Base(src))
				if err := copyFile(src, dst); err != nil {
					return fmt.Errorf("copy gentx to target: %w", err)
				}
				uio.LogInfo("  -> %s", filepath.Base(dst))
			}

			uio.LogStep("Collecting gentx into genesis...")
			docker := dkrcmd.New(dkrcmd.WithContainer(global.Container), dkrcmd.WithDataDir(dataDir))

			collectArgs := []string{"umeshnode", "genesis", "collect-gentxs", "--home", global.Home}
			if chainID != "" {
				collectArgs = append(collectArgs, "--chain-id", chainID)
			}

			out, err := docker.Exec(nil, collectArgs...)
			if err != nil {
				return fmt.Errorf("collect-gentxs: %w", err)
			}
			uio.LogSuccess("Genesis collected successfully")
			if len(out) > 0 {
				uio.Print(string(out))
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&repoURL, "repo", "", "URL to ZIP archive containing gentx JSON files")
	f.StringVar(&dataDir, "data-dir", "", "Host data directory (contains config/gentx)")
	f.StringVar(&chainID, "chain-id", "", "Chain ID for collect-gentxs (optional)")
	return cmd
}

func downloadGentxFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, resp.Body)
	return err
}

func extractGentxFiles(zipPath, extractDir string) ([]string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	var files []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if !strings.HasSuffix(name, "-gentx.json") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s in zip: %w", f.Name, err)
		}

		destPath := filepath.Join(extractDir, name)
		dest, err := os.Create(destPath)
		if err != nil {
			_ = rc.Close()
			return nil, fmt.Errorf("create %s: %w", destPath, err)
		}

		if _, err := io.Copy(dest, rc); err != nil {
			_ = dest.Close()
			_ = rc.Close()
			return nil, fmt.Errorf("extract %s: %w", name, err)
		}
		_ = dest.Close()
		_ = rc.Close()

		files = append(files, destPath)
	}
	return files, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// newGenesisPlanCmd creates the `umeshctl genesis plan` command.
func newGenesisPlanCmd() *cobra.Command {
	var configPath string
	var dryRun, force, keepKeys, autoPassword bool
	var keyringPasswordFile string
	var keyringPasswordStdin bool
	var keyringPasswordExec string

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Execute a genesis plan from YAML config",
		Long: `Execute a production genesis plan from a declarative YAML configuration file.

The plan defines tokenomics, allocations, vesting, module parameters, and soft launch settings.
This is the recommended way to create genesis for testnet and mainnet deployments.

Examples:
  umeshctl genesis plan --config genesis-plan.yaml --data-dir ./data-validator --auto-password
  umeshctl genesis plan --config genesis-plan.yaml --keyring-password-file secrets/validator-keyring.password
  umeshctl genesis plan --config genesis-plan.yaml --dry-run
  umeshctl genesis plan --config genesis-plan.yaml --force
  umeshctl genesis plan --config genesis-plan.yaml --force --keep-keys`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				return fmt.Errorf("--config is required")
			}
			if keepKeys && !force {
				return fmt.Errorf("--keep-keys requires --force")
			}
			if force && !dryRun {
				ok, err := uio.Confirm("--force will overwrite the existing genesis state. Proceed?", force)
				if err != nil {
					return err
				}
				if !ok {
					uio.LogInfo("Aborted.")
					return nil
				}
			}

			plan, err := nodeinit.ParsePlan(configPath)
			if err != nil {
				return fmt.Errorf("parse plan: %w", err)
			}

			nodeinit.SetHome(global.DataDir)
			nodeinit.ForceReinit = force
			nodeinit.KeepKeys = keepKeys

			pass, err := resolveKeyringPassword(cmd, autoPassword)
			if err != nil {
				return err
			}
			if pass == "" {
				uio.LogWarning("No keyring password provided. Key creation operations may fail if a password is required.")
			}
			nodeinit.SetKeyringPass(pass)

			return nodeinit.ExecutePlan(plan, dryRun)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to genesis plan YAML file (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate plan without executing")
	cmd.Flags().BoolVar(&force, "force", false, "Force reinitialization even if genesis.json exists")
	cmd.Flags().BoolVar(&keepKeys, "keep-keys", false, "Preserve validator identity (consensus + P2P keys) and reset chain state when regenerating with --force. For node init, use 'init <role> --keep-keys' to preserve node identity keys instead.")
	cmd.Flags().StringVarP(&keyringPasswordFile, "keyring-password-file", "p", "", "Read keyring password from file (alias: -p)")
	cmd.Flags().BoolVar(&keyringPasswordStdin, "keyring-password-stdin", false, "Read keyring password from stdin")
	cmd.Flags().StringVar(&keyringPasswordExec, "keyring-password-exec", "", "Execute command and read keyring password from stdout")
	if cmd.Flags().Lookup("auto-password") == nil {
		cmd.Flags().BoolVar(&autoPassword, "auto-password", false, "Generate a random keyring password and save it to disk")
	}
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.RegisterFlagCompletionFunc("config", completeYAMLFiles())

	return cmd
}

// newGenesisReportCmd creates the `umeshctl genesis report` command.
func newGenesisReportCmd() *cobra.Command {
	var configPath, output string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate allocation report from plan",
		Long: `Generate a report of token allocations from a genesis plan without executing it.

Examples:
  umeshctl genesis report --config genesis-plan.yaml
  umeshctl genesis report --config genesis-plan.yaml --output json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				return fmt.Errorf("--config is required")
			}

			plan, err := nodeinit.ParsePlan(configPath)
			if err != nil {
				return fmt.Errorf("parse plan: %w", err)
			}

			if err := nodeinit.ValidatePlan(plan); err != nil {
				return fmt.Errorf("plan validation failed: %w", err)
			}

			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			return nodeinit.PrintPlanReport(plan, string(format))
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to genesis plan YAML file (required)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	// Deprecated alias
	cmd.Flags().StringVar(&output, "format", "table", "Output format: table, text, json, yaml, yml (deprecated, use --output)")
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	_ = cmd.RegisterFlagCompletionFunc("format", completeOutputFormats())
	_ = cmd.Flags().MarkDeprecated("format", "use --output instead")

	return cmd
}

// newGenesisValidatePlanCmd creates the `umeshctl genesis validate-plan` command.
func newGenesisValidatePlanCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "validate-plan",
		Short: "Validate a genesis plan without executing",
		Long: `Validate a genesis plan YAML file for correctness.

Checks allocation percentages sum to 100%, validation rules, and module parameters.

Example:
  umeshctl genesis validate-plan --config genesis-plan.yaml`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				return fmt.Errorf("--config is required")
			}

			plan, err := nodeinit.ParsePlan(configPath)
			if err != nil {
				return fmt.Errorf("parse plan: %w", err)
			}

			if err := nodeinit.ValidatePlan(plan); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			uio.LogSuccess("Plan is valid")
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to genesis plan YAML file (required)")
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.RegisterFlagCompletionFunc("config", completeYAMLFiles())

	return cmd
}

// newGenesisCollectGentxCmd creates the `umeshctl genesis collect` command.

// newGenesisAddAccountCmd creates the `umeshctl genesis add-account` command.
func newGenesisAddAccountCmd() *cobra.Command {
	var keyName, addr, mnemonic, allocType, amount, endTime, startTime, moduleName string
	var keyringPasswordFile string
	var keyringPasswordStdin bool
	var keyringPasswordExec string

	cmd := &cobra.Command{
		Use:   "add-account",
		Short: "Add a single account to genesis (incremental)",
		Long: `Add a single account to the genesis document incrementally.

Useful for adding individual accounts after initial genesis creation.
Supports base accounts, vesting accounts, and module accounts.

Examples:
  umeshctl genesis add-account --key-name advisor --type base --amount 50000000000000uumesh --auto-password
  umeshctl genesis add-account --key-name advisor --type delayed_vesting --amount 50000000000000uumesh --end-time 2027-08-15T00:00:00Z --keyring-password-file secrets/keyring.password
  umeshctl genesis add-account --address umesh1... --type base --amount 1000000uumesh
  umeshctl genesis add-account --key-name distr --type module_account --module-name distribution --amount 1000000uumesh`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if amount == "" {
				return fmt.Errorf("--amount is required")
			}
			if keyName == "" && addr == "" {
				return fmt.Errorf("--key-name or --address is required")
			}
			if keyName != "" && addr != "" {
				return fmt.Errorf("--key-name and --address are mutually exclusive")
			}

			typ, err := mapAccountType(allocType)
			if err != nil {
				return err
			}

			denom := extractDenom(amount)
			if denom == "" {
				return fmt.Errorf("--amount must include a denom suffix (e.g. 5000000uumesh)")
			}
			bigAmount, ok := new(big.Int).SetString(nodeinit.ExtractAmount(amount), 10)
			if !ok || bigAmount.Sign() <= 0 {
				return fmt.Errorf("invalid amount: %s", amount)
			}

			switch typ {
			case "delayed_vesting":
				if endTime == "" {
					return fmt.Errorf("--end-time is required for delayed_vesting")
				}
			case "continuous_vesting":
				if startTime == "" || endTime == "" {
					return fmt.Errorf("--start-time and --end-time are required for continuous_vesting")
				}
			case "module_account":
				if moduleName == "" {
					return fmt.Errorf("--module-name is required for module_account")
				}
			}
			if startTime != "" {
				if _, err := time.Parse(time.RFC3339, startTime); err != nil {
					return fmt.Errorf("--start-time must be an RFC3339 timestamp: %w", err)
				}
			}
			if endTime != "" {
				if _, err := time.Parse(time.RFC3339, endTime); err != nil {
					return fmt.Errorf("--end-time must be an RFC3339 timestamp: %w", err)
				}
			}

			pass, err := resolveKeyringPassword(cmd, false)
			if err != nil {
				return err
			}
			nodeinit.SetKeyringPass(pass)

			d := dkrcmd.New(
				dkrcmd.WithImage(global.Image),
				dkrcmd.WithHome(global.Home),
				dkrcmd.WithDataDir(global.DataDir),
				dkrcmd.WithBackupsDir(nodeinit.BackupsDir(global.DataDir)),
			)

			alloc := nodeinit.Allocation{
				Name:       keyName,
				KeyName:    keyName,
				Address:    addr,
				Mnemonic:   mnemonic,
				Type:       typ,
				ModuleName: moduleName,
				Vesting: &nodeinit.VestingConfig{
					StartTime: startTime,
					EndTime:   endTime,
				},
			}

			switch alloc.Type {
			case "base_account":
				err = nodeinit.AddSingleBaseAccount(d, alloc, bigAmount, denom)
			case "delayed_vesting":
				err = nodeinit.AddSingleDelayedVesting(d, alloc, bigAmount, denom)
			case "continuous_vesting":
				err = nodeinit.AddSingleContinuousVesting(d, alloc, bigAmount, denom)
			case "module_account":
				err = nodeinit.AddSingleModuleAccount(d, alloc, bigAmount, denom)
			default:
				return fmt.Errorf("unsupported account type: %s", allocType)
			}

			if err != nil {
				return fmt.Errorf("add account failed: %w", err)
			}

			uio.LogSuccess("Account added successfully")
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&keyName, "key-name", "", "Key name (generates new if not exists)")
	f.StringVar(&addr, "address", "", "Use existing address (no key creation)")
	f.StringVar(&mnemonic, "mnemonic", "", "Recover key from mnemonic")
	f.StringVar(&allocType, "type", "base", "Account type: base, delayed_vesting, continuous_vesting, module_account")
	f.StringVar(&amount, "amount", "", "Amount with denom (e.g. 5000000uumesh)")
	f.StringVar(&endTime, "end-time", "", "Vesting end time (RFC3339)")
	f.StringVar(&startTime, "start-time", "", "Vesting start time (RFC3339)")
	f.StringVar(&moduleName, "module-name", "", "Module name for module_account type (e.g. distribution, staking)")
	f.StringVarP(&keyringPasswordFile, "keyring-password-file", "p", "", "Read keyring password from file (alias: -p)")
	f.BoolVar(&keyringPasswordStdin, "keyring-password-stdin", false, "Read keyring password from stdin")
	f.StringVar(&keyringPasswordExec, "keyring-password-exec", "", "Execute command and read keyring password from stdout")

	return cmd
}

// newGenesisAddValidatorCmd creates the `umeshctl genesis add-validator` command.
func newGenesisAddValidatorCmd() *cobra.Command {
	var keyName, moniker, selfDelegation, denom, chainID, externalIP, commissionRate string
	var keyringPasswordFile string
	var keyringPasswordStdin bool
	var keyringPasswordExec string

	cmd := &cobra.Command{
		Use:   "add-validator",
		Short: "Add a validator to genesis (for Mainnet Ritual)",
		Long: `Add a validator to the genesis document incrementally.

Creates a validator key, adds genesis account with self-delegation, and generates gentx.
Used in Mainnet Ritual flow where validators submit their own gentx.

Examples:
  umeshctl genesis add-validator --key-name validator-2 --moniker "My Node" --self-delegation 100000000000000uumesh --chain-id umesh-1 --auto-password
  umeshctl genesis add-validator --key-name validator-2 --self-delegation 50000000000000uumesh --denom uumesh --chain-id umesh-1 --keyring-password-file secrets/keyring.password`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if selfDelegation == "" {
				return fmt.Errorf("--self-delegation is required")
			}
			if chainID == "" {
				return fmt.Errorf("--chain-id is required")
			}
			if keyName == "" {
				keyName = "validator"
			}
			if moniker == "" {
				moniker = keyName
			}
			if denom == "" {
				denom = "uumesh"
			}
			if err := nodeinit.ValidateAmount(selfDelegation, denom); err != nil {
				return fmt.Errorf("--self-delegation: %w", err)
			}

			pass, err := resolveKeyringPassword(cmd, false)
			if err != nil {
				return err
			}
			nodeinit.SetKeyringPass(pass)

			d := dkrcmd.New(
				dkrcmd.WithImage(global.Image),
				dkrcmd.WithHome(global.Home),
				dkrcmd.WithDataDir(global.DataDir),
				dkrcmd.WithBackupsDir(nodeinit.BackupsDir(global.DataDir)),
			)

			// Resolve or generate key
			valAlloc := nodeinit.Allocation{
				Name:    keyName,
				KeyName: keyName,
			}
			address, _, _, err := nodeinit.ResolveKeyPublic(d, valAlloc)
			if err != nil {
				return fmt.Errorf("resolve key: %w", err)
			}

			// Add genesis account
			amount := nodeinit.ExtractAmount(selfDelegation)
			coinAmount := amount + denom
			_, err = d.RunMount(nil, "genesis", "add-genesis-account", address, coinAmount,
				"--home", global.Home)
			if err != nil {
				return fmt.Errorf("add-genesis-account: %w", err)
			}

			// Generate gentx
			gentxArgs := []string{"genesis", "gentx", keyName,
				coinAmount,
				"--chain-id", chainID,
				"--keyring-backend", "file",
				"--keyring-dir", global.Home + "/keyring",
				"--home", global.Home,
				"--moniker", moniker,
			}
			if externalIP != "" {
				gentxArgs = append(gentxArgs, "--ip", externalIP)
			}
			if commissionRate != "" {
				gentxArgs = append(gentxArgs, "--commission-rate", commissionRate)
			}

			_, err = d.RunMount(strings.NewReader(nodeinit.GetKeyringPass()+"\n"), gentxArgs...)
			if err != nil {
				return fmt.Errorf("gentx: %w", err)
			}

			uio.LogSuccess("Validator %s added, gentx generated", keyName)
			uio.LogInfo("Run 'umeshctl genesis collect-gentx' to include in genesis")
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&keyName, "key-name", "validator", "Validator key name")
	f.StringVar(&moniker, "moniker", "", "Validator moniker (defaults to key-name)")
	f.StringVar(&selfDelegation, "self-delegation", "", "Self-delegation amount (e.g. 100000000000000uumesh)")
	f.StringVar(&denom, "denom", "uumesh", "Denom")
	f.StringVar(&chainID, "chain-id", "", "Chain ID (required)")
	f.StringVar(&externalIP, "ip", "", "External IP address")
	f.StringVar(&commissionRate, "commission-rate", "0.10", "Commission rate")
	f.StringVarP(&keyringPasswordFile, "keyring-password-file", "p", "", "Read keyring password from file (alias: -p)")
	f.BoolVar(&keyringPasswordStdin, "keyring-password-stdin", false, "Read keyring password from stdin")
	f.StringVar(&keyringPasswordExec, "keyring-password-exec", "", "Execute command and read keyring password from stdout")

	return cmd
}

// mapAccountType maps CLI type string to internal allocation type.
func mapAccountType(cliType string) (string, error) {
	switch cliType {
	case "base", "base_account":
		return "base_account", nil
	case "delayed_vesting":
		return "delayed_vesting", nil
	case "continuous_vesting":
		return "continuous_vesting", nil
	case "module", "module_account":
		return "module_account", nil
	default:
		return "", fmt.Errorf("unsupported account type %q (supported: base, delayed_vesting, continuous_vesting, module_account)", cliType)
	}
}

// extractDenom extracts denom from amount string (e.g. "5000uumesh" -> "uumesh").
func extractDenom(s string) string {
	i := len(s) - 1
	for i >= 0 && (s[i] < '0' || s[i] > '9') {
		i--
	}
	return s[i+1:]
}
