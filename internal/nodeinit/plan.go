package nodeinit

import (
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/opscores/umesh-cli/internal/nodeinfo"
	"github.com/opscores/umesh-cli/internal/tune"
	"github.com/opscores/umesh-cli/internal/uio"
	"gopkg.in/yaml.v3"
)

// ParsePlan reads and parses a genesis plan from a YAML file.
func ParsePlan(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan file: %w", err)
	}
	var plan Plan
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parse plan YAML: %w", err)
	}
	return &plan, nil
}

// ValidatePlan validates a genesis plan against allocation rules. It also
// normalizes the plan in place so downstream steps (report, execution) see a
// deterministic document:
//   - genesis_time "now"/"" is resolved once to a fixed RFC3339 timestamp;
//   - a vesting start_time earlier than genesis_time is clamped to genesis_time
//     (the chain cannot exist before it launches, so the operator's intent is
//     taken as "vesting begins at launch") and a warning is emitted.
func ValidatePlan(plan *Plan) error {
	plan.Chain.GenesisTime = resolveGenesisTime(plan.Chain.GenesisTime)
	if err := validateChain(&plan.Chain); err != nil {
		return err
	}
	if err := validateTokenomics(&plan.Tokenomics, plan.Chain.GenesisTime); err != nil {
		return err
	}
	if err := validateModules(&plan.Modules, plan.Chain.Denom); err != nil {
		return err
	}
	return nil
}

func validateChain(c *ChainConfig) error {
	if c.ChainID == "" {
		return fmt.Errorf("chain_id is required")
	}
	if c.Moniker == "" {
		return fmt.Errorf("moniker is required")
	}
	if c.Denom == "" {
		return fmt.Errorf("denom is required")
	}
	if c.Decimals < 0 || c.Decimals > 18 {
		return fmt.Errorf("decimals must be between 0 and 18, got %d", c.Decimals)
	}
	if c.GenesisTime != "" {
		if _, err := parseGenesisTime(resolveGenesisTime(c.GenesisTime)); err != nil {
			return fmt.Errorf("invalid genesis_time: %w", err)
		}
	}
	if err := validateConsensusParams(&c.Consensus); err != nil {
		return err
	}
	return nil
}

// validateConsensusParams checks user-supplied consensus parameters. Zero
// values are allowed and fall back to DefaultConsensusParams at patch time.
func validateConsensusParams(p *ConsensusParams) error {
	if p.BlockMaxBytes < 0 {
		return fmt.Errorf("consensus.block_max_bytes must be >= 0, got %d", p.BlockMaxBytes)
	}
	if p.BlockMaxGas < -1 {
		return fmt.Errorf("consensus.block_max_gas must be -1 or >= 0, got %d", p.BlockMaxGas)
	}
	if p.TimeIotaMs < 0 {
		return fmt.Errorf("consensus.time_iota_ms must be >= 0, got %d", p.TimeIotaMs)
	}
	if p.EvidenceMaxAgeNumBlocks < 0 {
		return fmt.Errorf("consensus.evidence_max_age_num_blocks must be >= 0, got %d", p.EvidenceMaxAgeNumBlocks)
	}
	if p.EvidenceMaxAgeDuration != "" {
		if _, err := time.ParseDuration(p.EvidenceMaxAgeDuration); err != nil {
			return fmt.Errorf("invalid consensus.evidence_max_age_duration %q: %w", p.EvidenceMaxAgeDuration, err)
		}
	}
	if p.EvidenceMaxBytes < 0 {
		return fmt.Errorf("consensus.evidence_max_bytes must be >= 0, got %d", p.EvidenceMaxBytes)
	}
	for _, k := range p.ValidatorPubKeyTypes {
		if k != "ed25519" {
			return fmt.Errorf("consensus.validator_pub_key_types: unsupported key type %q (only ed25519 is supported)", k)
		}
	}
	if p.Authority != "" {
		if !validBech32Address(p.Authority) {
			return fmt.Errorf("consensus.authority: %q is not a valid bech32 address", p.Authority)
		}
	}
	return nil
}

// validBech32Address reports whether s looks like a bech32 account address:
// "<hrp>1<32-char data with 6-char checksum>" for a 20-byte key. It performs a
// structural check without pulling in a bech32 dependency.
func validBech32Address(s string) bool {
	if len(s) < 8 || len(s) > 100 {
		return false
	}
	sep := strings.Index(s, "1")
	if sep < 1 || sep+1 >= len(s) {
		return false
	}
	// Human-readable part: lowercase alphanumeric.
	for _, r := range s[:sep] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	// Data part: bech32 charset (lowercase alphanumeric, excluding b, i, o, 1).
	for _, r := range s[sep+1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
		if r == 'b' || r == 'i' || r == 'o' || r == '1' {
			return false
		}
	}
	return true
}

func validateTokenomics(t *Tokenomics, genesisTime string) error {
	if t.TotalSupply == "" {
		return fmt.Errorf("total_supply is required")
	}
	totalSupply, ok := new(big.Int).SetString(t.TotalSupply, 10)
	if !ok {
		return fmt.Errorf("invalid total_supply: %q", t.TotalSupply)
	}
	if totalSupply.Sign() <= 0 {
		return fmt.Errorf("total_supply must be positive")
	}
	if len(t.Allocations) == 0 {
		return fmt.Errorf("at least one allocation is required")
	}

	// Set defaults for validation if not specified
	v := t.Validation
	if v.MaxSingleAllocation == 0 {
		v.MaxSingleAllocation = 25.0
	}
	if v.MaxInsiderAllocation == 0 {
		v.MaxInsiderAllocation = 45.0
	}
	if v.MinValidatorCount == 0 {
		v.MinValidatorCount = 1
	}

	var totalPercentage float64
	var insiderPercentage float64
	var validatorCount int

	for i, a := range t.Allocations {
		if err := validateAllocationType(a.Type); err != nil {
			return fmt.Errorf("allocation %d (%s): %w", i, a.Name, err)
		}
		if a.Name == "" {
			return fmt.Errorf("allocation %d: name is required", i)
		}
		if a.Percentage <= 0 {
			return fmt.Errorf("allocation %s: percentage must be positive", a.Name)
		}
		if a.Percentage > v.MaxSingleAllocation {
			return fmt.Errorf("allocation %s: percentage %.2f%% exceeds max single allocation %.2f%%",
				a.Name, a.Percentage, v.MaxSingleAllocation)
		}
		totalPercentage += a.Percentage

		if IsInsider(a.Name) {
			insiderPercentage += a.Percentage
		}

		if a.Type == "validator_set" {
			if len(a.Validators) == 0 {
				return fmt.Errorf("allocation %s: validator_set requires at least one validator", a.Name)
			}
			validatorCount += len(a.Validators)
			if err := validateValidators(a.Validators); err != nil {
				return fmt.Errorf("allocation %s: %w", a.Name, err)
			}
		}

		// Validate vesting config for vesting types
		switch a.Type {
		case "delayed_vesting", "continuous_vesting":
			if a.Vesting == nil {
				return fmt.Errorf("allocation %s: vesting config required for type %s", a.Name, a.Type)
			}
			if err := validateVestingTimeRange(a.Vesting.EndTime, a.Vesting.StartTime); err != nil {
				return fmt.Errorf("allocation %s: %w", a.Name, err)
			}
			// Tokens must not begin unlocking before the chain exists. When the
			// plan sets a vesting start before the (resolved) genesis_time, treat
			// the intent as "vesting begins at launch": clamp the start to
			// genesis_time and warn instead of failing.
			if a.Vesting.StartTime != "" && genesisTime != "" {
				startUnix, err := parseUnixTime(a.Vesting.StartTime)
				if err != nil {
					return fmt.Errorf("allocation %s: invalid vesting start_time: %w", a.Name, err)
				}
				resolvedGenesis := resolveGenesisTime(genesisTime)
				genUnix, err := parseUnixTime(resolvedGenesis)
				if err != nil {
					return fmt.Errorf("allocation %s: invalid genesis_time: %w", a.Name, err)
				}
				if startUnix < genUnix {
					uio.LogWarning("allocation %s: vesting start_time (%s) was before genesis_time (%s); adjusted to genesis_time",
						a.Name, a.Vesting.StartTime, resolvedGenesis)
					a.Vesting.StartTime = resolvedGenesis
				}
			}
		}
	}

	// Check total percentage (allow small floating point tolerance)
	if totalPercentage < 99.99 || totalPercentage > 100.01 {
		return fmt.Errorf("total allocation percentage must be 100%%, got %.2f%%", totalPercentage)
	}

	// Check insider allocation
	if insiderPercentage > v.MaxInsiderAllocation {
		return fmt.Errorf("insider allocation %.2f%% exceeds max %.2f%%",
			insiderPercentage, v.MaxInsiderAllocation)
	}

	// Check validator count
	if validatorCount < v.MinValidatorCount {
		return fmt.Errorf("validator count %d below minimum %d", validatorCount, v.MinValidatorCount)
	}

	return nil
}

func validateValidators(validators []ValidatorConfig) error {
	for i, v := range validators {
		if v.Name == "" {
			return fmt.Errorf("validator %d: name is required", i)
		}
		if v.SelfDelegation == "" {
			return fmt.Errorf("validator %s: self_delegation is required", v.Name)
		}
		if v.CommissionRate == "" {
			return fmt.Errorf("validator %s: commission_rate is required", v.Name)
		}
	}
	return nil
}

// parseDec parses a decimal string (e.g., "0.670000000000000000") into a big.Float.
func parseDec(s string) (*big.Float, error) {
	f, ok := new(big.Float).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid decimal %q", s)
	}
	return f, nil
}

// validateGovParams validates governance module parameters.
func validateGovParams(p GovParams) error {
	// expedited_threshold must be > threshold (SDK invariant)
	if p.ExpeditedThreshold != "" && p.Threshold != "" {
		expThresh, err := parseDec(p.ExpeditedThreshold)
		if err != nil {
			return fmt.Errorf("gov.expedited_threshold: %w", err)
		}
		thresh, err := parseDec(p.Threshold)
		if err != nil {
			return fmt.Errorf("gov.threshold: %w", err)
		}
		if expThresh.Cmp(thresh) <= 0 {
			return fmt.Errorf("gov.expedited_threshold must be > gov.threshold")
		}
	}

	// proposal_cancel_ratio must be in (0, 1]
	if p.ProposalCancelRatio != "" {
		ratio, err := parseDec(p.ProposalCancelRatio)
		if err != nil {
			return fmt.Errorf("gov.proposal_cancel_ratio: %w", err)
		}
		if ratio.Sign() <= 0 || ratio.Cmp(big.NewFloat(1)) > 0 {
			return fmt.Errorf("gov.proposal_cancel_ratio must be in (0, 1]")
		}
	}

	// min_deposit_ratio must be in (0, 1]
	if p.MinDepositRatio != "" {
		ratio, err := parseDec(p.MinDepositRatio)
		if err != nil {
			return fmt.Errorf("gov.min_deposit_ratio: %w", err)
		}
		if ratio.Sign() <= 0 || ratio.Cmp(big.NewFloat(1)) > 0 {
			return fmt.Errorf("gov.min_deposit_ratio must be in (0, 1]")
		}
	}

	// expedited_min_deposit must be > min_deposit (SDK invariant)
	if p.ExpeditedMinDeposit != "" && p.MinDeposit != "" {
		expDeposit, ok := new(big.Int).SetString(p.ExpeditedMinDeposit, 10)
		if !ok {
			return fmt.Errorf("gov.expedited_min_deposit: invalid integer")
		}
		minDeposit, ok := new(big.Int).SetString(p.MinDeposit, 10)
		if !ok {
			return fmt.Errorf("gov.min_deposit: invalid integer")
		}
		if expDeposit.Cmp(minDeposit) <= 0 {
			return fmt.Errorf("gov.expedited_min_deposit must be > gov.min_deposit")
		}
	}

	return nil
}

func validateModules(m *Modules, denom string) error {
	// Validate staking params
	if m.Staking.BondDenom == "" {
		m.Staking.BondDenom = denom
	}
	if m.Staking.MaxValidators <= 0 {
		m.Staking.MaxValidators = 100
	}

	// Validate gov params
	if err := validateGovParams(m.Gov); err != nil {
		return err
	}

	// Validate epochs params
	seen := map[string]bool{}
	for i, e := range m.Epochs.Epochs {
		if e.Identifier == "" {
			return fmt.Errorf("epochs[%d]: identifier is required", i)
		}
		if seen[e.Identifier] {
			return fmt.Errorf("epochs[%d]: duplicate identifier %q", i, e.Identifier)
		}
		seen[e.Identifier] = true
		if e.Duration == "" {
			return fmt.Errorf("epochs[%d] (%s): duration is required", i, e.Identifier)
		}
		d, err := time.ParseDuration(e.Duration)
		if err != nil {
			return fmt.Errorf("epochs[%d] (%s): invalid duration %q: %w", i, e.Identifier, e.Duration, err)
		}
		if d <= 0 {
			return fmt.Errorf("epochs[%d] (%s): duration must be positive, got %s", i, e.Identifier, e.Duration)
		}
		if e.StartTime != "" {
			if _, err := parseGenesisTime(e.StartTime); err != nil {
				return fmt.Errorf("epochs[%d] (%s): invalid start_time: %w", i, e.Identifier, err)
			}
		}
	}

	// Validate protocolpool params
	if m.ProtocolPool.DistributionFrequency == 0 && len(m.ProtocolPool.EnabledDistributionDenoms) == 0 && len(m.ProtocolPool.ContinuousFunds) == 0 {
		return nil
	}
	if m.ProtocolPool.DistributionFrequency == 0 {
		return fmt.Errorf("protocolpool.distribution_frequency must be > 0")
	}
	for j, d := range m.ProtocolPool.EnabledDistributionDenoms {
		if d == "" {
			return fmt.Errorf("protocolpool.enabled_distribution_denoms[%d]: denom is required", j)
		}
	}
	for i, f := range m.ProtocolPool.ContinuousFunds {
		if f.Recipient == "" {
			return fmt.Errorf("protocolpool.continuous_funds[%d]: recipient is required", i)
		}
		if !validBech32Address(f.Recipient) {
			return fmt.Errorf("protocolpool.continuous_funds[%d]: %q is not a valid bech32 address", i, f.Recipient)
		}
		p, ok := new(big.Float).SetString(f.Percentage)
		if !ok || p.Sign() <= 0 || p.Cmp(big.NewFloat(1)) > 0 {
			return fmt.Errorf("protocolpool.continuous_funds[%d]: percentage %q must be a decimal in (0, 1]", i, f.Percentage)
		}
		if f.Expiry != "" {
			if _, err := parseGenesisTime(f.Expiry); err != nil {
				return fmt.Errorf("protocolpool.continuous_funds[%d]: invalid expiry: %w", i, err)
			}
		}
	}
	return nil
}

// ExecutePlan executes the genesis plan.
func ExecutePlan(plan *Plan, dryRun bool) error {
	if err := ValidatePlan(plan); err != nil {
		return fmt.Errorf("plan validation failed: %w", err)
	}

	if dryRun {
		uio.LogInfo("DRY RUN: plan validation passed")
		return printPlanSummary(plan)
	}

	// Check if already initialized
	if err := AbortIfInitialized(); err != nil {
		return err
	}

	d := docker()
	if err := d.Preflight(); err != nil {
		return err
	}

	// Step 1: Initialize node
	uio.LogStep("Initializing node...")

	// When --keep-keys is requested, capture the node identity before
	// `umeshd init --overwrite` replaces the consensus and P2P keys.
	var keyBackup *identityKeys
	if KeepKeys {
		kb, err := captureIdentityKeys(Home())
		if err != nil {
			return fmt.Errorf("capture node keys failed: %w", err)
		}
		keyBackup = kb
	}

	if err := umeshdInit(d, plan.Chain.Moniker, plan.Chain.ChainID, Home(), false); err != nil {
		return fmt.Errorf("init failed: %w", err)
	}

	// Restore the captured identity and reset the blockchain state so the
	// regenerated genesis starts cleanly with the same validator identity.
	if KeepKeys {
		if err := keyBackup.restore(Home()); err != nil {
			return fmt.Errorf("restore node keys failed: %w", err)
		}
		if err := resetChainState(Home()); err != nil {
			return fmt.Errorf("reset chain state failed: %w", err)
		}
		uio.LogInfo("Preserved validator identity (consensus + node keys) and reset chain state")
	}

	// Step 1b: Apply plan genesis_time. `umeshd init` stamps genesis with the
	// current time, so without this the configured launch time would be
	// silently ignored in the resulting genesis.json. The special value "now"
	// (or empty) makes the chain start immediately.
	if plan.Chain.GenesisTime != "" {
		gt := resolveGenesisTime(plan.Chain.GenesisTime)
		uio.LogStep("Setting genesis_time...")
		if err := SetGenesisTime(gt); err != nil {
			return fmt.Errorf("set genesis_time failed: %w", err)
		}
	}

	// Step 2: Apply tuning
	uio.LogStep("Applying tuning profile...")
	if err := tune.Apply(ConfigDir(), tune.RoleGenesis, tune.Options{
		Moniker:     plan.Chain.Moniker,
		Environment: "production",
		Denom:       plan.Chain.Denom,
		MinGasPrice: "0.0025",
		ExternalAddress: planValidatorExternalAddress(plan),
	}); err != nil {
		return fmt.Errorf("tune failed: %w", err)
	}

	// Advertise the genesis validator's P2P address in config.toml. Without
	// p2p.external_address the node running in its container would advertise
	// its bridge IP, which peers on other VPS hosts cannot dial.
	if addr := planValidatorExternalAddress(plan); addr != "" {
		if err := setExternalAddress(addr); err != nil {
			return fmt.Errorf("set external address failed: %w", err)
		}
	}

	// Step 3: Patch denom
	uio.LogStep("Patching denom...")
	if err := patchDenom(plan.Chain.Denom); err != nil {
		return fmt.Errorf("patch denom failed: %w", err)
	}

	// Step 4: Add bank metadata
	uio.LogStep("Adding bank metadata...")
	if err := addBankMetadataFromPlan(plan); err != nil {
		return fmt.Errorf("add bank metadata failed: %w", err)
	}

	// Step 5: Patch consensus params
	uio.LogStep("Patching consensus parameters...")
	if err := patchConsensusParams(plan.Chain.Consensus); err != nil {
		return fmt.Errorf("patch consensus params failed: %w", err)
	}

	// Step 6: Create accounts from allocations
	uio.LogStep("Creating allocations...")
	resolvedAllocations, err := createAllocations(d, plan)
	if err != nil {
		return fmt.Errorf("create allocations failed: %w", err)
	}

	// Step 7: Patch module params
	uio.LogStep("Patching module parameters...")
	if err := patchModuleParams(plan); err != nil {
		return fmt.Errorf("patch module params failed: %w", err)
	}

	// Step 8: Handle soft launch
	uio.LogStep("Configuring soft launch...")
	if err := handleSoftLaunch(plan); err != nil {
		return fmt.Errorf("soft launch config failed: %w", err)
	}

	// Step 9: Handle dust
	uio.LogStep("Handling dust...")
	if err := handleDust(plan, resolvedAllocations); err != nil {
		return fmt.Errorf("dust handling failed: %w", err)
	}

	// Step 10: Collect gentxs
	uio.LogStep("Collecting gentxs...")
	if err := collectGentxs(d); err != nil {
		return fmt.Errorf("collect gentxs failed: %w", err)
	}

	// Print validator commission summary table post-collection (Audit requirement M4)
	PrintGentxCommissionSummary(plan)

	// Step 11: Validate genesis
	uio.LogStep("Validating genesis...")
	if err := validateGenesis(d); err != nil {
		return fmt.Errorf("genesis validation failed: %w", err)
	}

	// Step 12: Write node info
	uio.LogStep("Writing node info...")
	if err := writePlanNodeInfo(plan); err != nil {
		return fmt.Errorf("write node info failed: %w", err)
	}

	// Step 13: Generate report
	uio.LogStep("Generating report...")
	if err := generateReport(plan, resolvedAllocations); err != nil {
		return fmt.Errorf("generate report failed: %w", err)
	}

	uio.LogSuccess("Genesis plan executed successfully!")
	return nil
}

func printPlanSummary(plan *Plan) error {
	uio.Print("Chain ID:    %s", plan.Chain.ChainID)
	uio.Print("Moniker:     %s", plan.Chain.Moniker)
	uio.Print("Denom:       %s", plan.Chain.Denom)
	uio.Print("Supply:      %s", plan.Tokenomics.TotalSupply)
	uio.Print("Allocations: %d", len(plan.Tokenomics.Allocations))
	return nil
}

// planValidatorExternalAddress returns the p2p.external_address of the first
// genesis validator declared in the plan (the node this host runs), or "" when
// the plan does not set one.
func planValidatorExternalAddress(plan *Plan) string {
	for _, alloc := range plan.Tokenomics.Allocations {
		if alloc.Type == "validator_set" && len(alloc.Validators) > 0 {
			return alloc.Validators[0].ExternalAddress
		}
	}
	return ""
}

func writePlanNodeInfo(plan *Plan) error {
	d := docker()
	validatorName := "validator"
	// Find the validator_set allocation and get the first validator name
	for _, alloc := range plan.Tokenomics.Allocations {
		if alloc.Type == "validator_set" && len(alloc.Validators) > 0 {
			validatorName = alloc.Validators[0].Name
			break
		}
	}
	genBytes, _ := os.ReadFile(GenesisFile())
	genSHA := SHA256Hex(genBytes)

	info := nodeinfo.Info{
		ChainID:            plan.Chain.ChainID,
		Mode:               "validator",
		ValidatorOperator:  valoperFromKeyring(d, validatorName, GetKeyringPass()),
		ValidatorAddress:   localValidatorAddr(Home()),
		NodeID:             localNodeID(Home()),
		KeyringBackend:     "file",
		GenesisTime:        extractGenesisTime(GenesisFile()),
		AutoAccountCreated: true,
		ValidatorReady:     1,
		GenesisSHA256:      genSHA,
	}
	return nodeinfo.Write(Home(), info)
}

// ExecutePlanFromEnv executes a plan using environment variables for secrets.
func ExecutePlanFromEnv(plan *Plan, keyringPass string, dryRun bool) error {
	// Store keyring password for use by key resolution
	SetKeyringPass(keyringPass)
	return ExecutePlan(plan, dryRun)
}
