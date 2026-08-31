package nodeinit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/opscores/umesh-cli/internal/uio"
)

// patchModuleParams patches all module parameters from the plan.
func patchModuleParams(plan *Plan) error {
	denom := plan.Chain.Denom
	m := plan.Modules

	enabled, err := enabledAppStateModules()
	if err != nil {
		return err
	}

	// Staking params
	if err := patchModuleParamsIfEnabled("staking", enabled, func() error {
		return patchStakingParams(m.Staking, denom)
	}); err != nil {
		return fmt.Errorf("staking params: %w", err)
	}

	// Distribution params
	if err := patchModuleParamsIfEnabled("distribution", enabled, func() error {
		return patchDistributionParams(m.Distribution)
	}); err != nil {
		return fmt.Errorf("distribution params: %w", err)
	}

	// Mint params
	if err := patchModuleParamsIfEnabled("mint", enabled, func() error {
		return patchMintParams(m.Mint, denom)
	}); err != nil {
		return fmt.Errorf("mint params: %w", err)
	}

	// Gov params
	if err := patchModuleParamsIfEnabled("gov", enabled, func() error {
		return patchGovParams(m.Gov, denom)
	}); err != nil {
		return fmt.Errorf("gov params: %w", err)
	}

	// Gov constitution (immutable on-chain document)
	if plan.Chain.Constitution != "" {
		if err := patchModuleParamsIfEnabled("gov", enabled, func() error {
			return patchJSONParam("app_state.gov.constitution", plan.Chain.Constitution)
		}); err != nil {
			return fmt.Errorf("gov constitution: %w", err)
		}
	}

	// Slashing params
	if err := patchModuleParamsIfEnabled("slashing", enabled, func() error {
		return patchSlashingParams(m.Slashing)
	}); err != nil {
		return fmt.Errorf("slashing params: %w", err)
	}

	// Bank params
	if err := patchModuleParamsIfEnabled("bank", enabled, func() error {
		return patchBankParams(m.Bank)
	}); err != nil {
		return fmt.Errorf("bank params: %w", err)
	}

	// Wasm params
	if err := patchModuleParamsIfEnabled("wasm", enabled, func() error {
		return patchWasmParams(m.Wasm)
	}); err != nil {
		return fmt.Errorf("wasm params: %w", err)
	}

	// Epochs params
	if err := patchModuleParamsIfEnabled("epochs", enabled, func() error {
		return patchEpochsParams(m.Epochs)
	}); err != nil {
		return fmt.Errorf("epochs params: %w", err)
	}

	// Protocolpool params
	if err := patchModuleParamsIfEnabled("protocolpool", enabled, func() error {
		return patchProtocolPoolParams(m.ProtocolPool)
	}); err != nil {
		return fmt.Errorf("protocolpool params: %w", err)
	}

	return nil
}

// patchModuleParamsIfEnabled runs patch when the module is present in the
// generated genesis app_state (i.e. compiled into umeshd). When the module is
// absent, the plan parameters are skipped with a warning so the resulting
// genesis.json is formed only from the modules the binary actually has.
func patchModuleParamsIfEnabled(module string, enabled map[string]bool, patch func() error) error {
	if enabled[module] {
		return patch()
	}
	uio.LogWarning("module %q is not compiled into umeshd; skipping its plan parameters", module)
	return nil
}

func patchStakingParams(p StakingParams, denom string) error {
	bondDenom := p.BondDenom
	if bondDenom == "" {
		bondDenom = denom
	}

	params := map[string]any{
		"max_validators":      p.MaxValidators,
		"bond_denom":          bondDenom,
		"historical_entries":  p.HistoricalEntries,
	}

	if p.MaxEntries > 0 {
		params["max_entries"] = p.MaxEntries
	}
	if p.UnbondingTime != "" {
		params["unbonding_time"] = p.UnbondingTime
	}
	if p.MinCommissionRate != "" {
		params["min_commission_rate"] = p.MinCommissionRate
	}
	for k, v := range params {
		path := fmt.Sprintf("app_state.staking.params.%s", k)
		if err := patchJSONParam(path, v); err != nil {
			return err
		}
	}
	return nil
}

func patchDistributionParams(p DistributionParams) error {
	params := map[string]any{}

	if p.CommunityTax != "" {
		params["community_tax"] = p.CommunityTax
	}
	params["withdraw_addr_enabled"] = p.WithdrawAddrEnabled

	for k, v := range params {
		path := fmt.Sprintf("app_state.distribution.params.%s", k)
		if err := patchJSONParam(path, v); err != nil {
			return err
		}
	}
	return nil
}

func patchMintParams(p MintParams, denom string) error {
	mintDenom := p.MintDenom
	if mintDenom == "" {
		mintDenom = denom
	}

	params := map[string]any{
		"mint_denom": mintDenom,
	}

	if p.InflationRateChange != "" {
		params["inflation_rate_change"] = p.InflationRateChange
	}
	if p.InflationMax != "" {
		params["inflation_max"] = p.InflationMax
	}
	if p.InflationMin != "" {
		params["inflation_min"] = p.InflationMin
	}
	if p.GoalBonded != "" {
		params["goal_bonded"] = p.GoalBonded
	}
	if p.BlocksPerYear != "" {
		params["blocks_per_year"] = p.BlocksPerYear
	}
	if p.MaxSupply != "" {
		params["max_supply"] = p.MaxSupply
	}

	for k, v := range params {
		path := fmt.Sprintf("app_state.mint.params.%s", k)
		if err := patchJSONParam(path, v); err != nil {
			return err
		}
	}
	return nil
}

func patchGovParams(p GovParams, denom string) error {
	params := map[string]any{}

	if p.MinDeposit != "" {
		params["min_deposit"] = []map[string]string{
			{"denom": denom, "amount": p.MinDeposit},
		}
	}
	if p.MaxDepositPeriod != "" {
		params["max_deposit_period"] = p.MaxDepositPeriod
	}
	if p.VotingPeriod != "" {
		params["voting_period"] = p.VotingPeriod
	}
	if p.Quorum != "" {
		params["quorum"] = p.Quorum
	}
	if p.Threshold != "" {
		params["threshold"] = p.Threshold
	}
	if p.VetoThreshold != "" {
		params["veto_threshold"] = p.VetoThreshold
	}
	if p.MinInitialDepositRatio != "" {
		params["min_initial_deposit_ratio"] = p.MinInitialDepositRatio
	}
	if p.ExpeditedMinDeposit != "" {
		params["expedited_min_deposit"] = []map[string]string{
			{"denom": denom, "amount": p.ExpeditedMinDeposit},
		}
	}
	if p.BurnVoteQuorum {
		params["burn_vote_quorum"] = true
	}
	if p.BurnProposalDepositPrevote {
		params["burn_proposal_deposit_prevote"] = true
	}
	if p.ExpeditedVotingPeriod != "" {
		params["expedited_voting_period"] = p.ExpeditedVotingPeriod
	}
	if p.ExpeditedThreshold != "" {
		params["expedited_threshold"] = p.ExpeditedThreshold
	}
	if p.ProposalCancelRatio != "" {
		params["proposal_cancel_ratio"] = p.ProposalCancelRatio
	}
	if p.ProposalCancelDest != "" {
		params["proposal_cancel_dest"] = p.ProposalCancelDest
	}
	if p.BurnVoteVeto {
		params["burn_vote_veto"] = true
	}
	if p.MinDepositRatio != "" {
		params["min_deposit_ratio"] = p.MinDepositRatio
	}
	if p.StartingProposalID != "" {
		params["starting_proposal_id"] = p.StartingProposalID
	}

	for k, v := range params {
		path := fmt.Sprintf("app_state.gov.params.%s", k)
		if err := patchJSONParam(path, v); err != nil {
			return err
		}
	}
	return nil
}

func patchSlashingParams(p SlashingParams) error {
	params := map[string]any{}

	if p.SignedBlocksWindow != "" {
		params["signed_blocks_window"] = p.SignedBlocksWindow
	}
	if p.MinSignedPerWindow != "" {
		params["min_signed_per_window"] = p.MinSignedPerWindow
	}
	if p.DowntimeJailDuration != "" {
		params["downtime_jail_duration"] = p.DowntimeJailDuration
	}
	if p.SlashFractionDoubleSign != "" {
		params["slash_fraction_double_sign"] = p.SlashFractionDoubleSign
	}
	if p.SlashFractionDowntime != "" {
		params["slash_fraction_downtime"] = p.SlashFractionDowntime
	}

	for k, v := range params {
		path := fmt.Sprintf("app_state.slashing.params.%s", k)
		if err := patchJSONParam(path, v); err != nil {
			return err
		}
	}
	return nil
}

func patchBankParams(p BankParams) error {
	params := map[string]any{
		"default_send_enabled": p.DefaultSendEnabled,
	}

	for k, v := range params {
		path := fmt.Sprintf("app_state.bank.params.%s", k)
		if err := patchJSONParam(path, v); err != nil {
			return err
		}
	}
	return nil
}

func patchWasmParams(p WasmParams) error {
	params := map[string]any{}

	// AccessType enum values are serialized with MarshalText: "Nobody",
	// "Everybody", "AnyOfAddresses" (capitalized). Lowercase values parse as
	// AccessTypeUnspecified and fail validation.
	if p.CodeUploadAccess != "" {
		params["code_upload_access"] = map[string]any{
			"permission": capitalizeAccessType(p.CodeUploadAccess),
		}
	}
	if p.InstantiateDefaultPermission != "" {
		params["instantiate_default_permission"] = capitalizeAccessType(p.InstantiateDefaultPermission)
	}

	for k, v := range params {
		path := fmt.Sprintf("app_state.wasm.params.%s", k)
		if err := patchJSONParam(path, v); err != nil {
			return err
		}
	}
	return nil
}

// patchEpochsParams writes the configured epoch timers into the x/epochs
// genesis state. Only operator-set fields (identifier, start_time, duration)
// are written; the SDK fills the runtime state during InitGenesis. When the
// plan leaves the list empty nothing is patched, keeping the chain defaults.
func patchEpochsParams(p EpochsParams) error {
	if len(p.Epochs) == 0 {
		return nil
	}

	epochs := make([]map[string]any, 0, len(p.Epochs))
	for _, e := range p.Epochs {
		entry := map[string]any{
			"identifier": e.Identifier,
			"duration":   e.Duration,
		}
		if e.StartTime != "" {
			entry["start_time"] = e.StartTime
		}
		epochs = append(epochs, entry)
	}

	if err := patchJSONParam("app_state.epochs.epochs", epochs); err != nil {
		return err
	}
	return nil
}

// patchProtocolPoolParams writes the x/protocolpool genesis configuration:
// module params (enabled_distribution_denoms, distribution_frequency) and
// continuous funds. This is separate from x/distribution's community pool
// (distribution.fee_pool), which is managed by the dust/allocation flows.
func patchProtocolPoolParams(p ProtocolPoolParams) error {
	if p.DistributionFrequency == 0 && len(p.EnabledDistributionDenoms) == 0 && len(p.ContinuousFunds) == 0 {
		return nil
	}

	if p.DistributionFrequency != 0 {
		if err := patchJSONParam("app_state.protocolpool.params.distribution_frequency", p.DistributionFrequency); err != nil {
			return err
		}
	}
	if len(p.EnabledDistributionDenoms) > 0 {
		if err := patchJSONParam("app_state.protocolpool.params.enabled_distribution_denoms", p.EnabledDistributionDenoms); err != nil {
			return err
		}
	}
	if len(p.ContinuousFunds) > 0 {
		funds := make([]map[string]any, 0, len(p.ContinuousFunds))
		for _, f := range p.ContinuousFunds {
			entry := map[string]any{
				"recipient":  f.Recipient,
				"percentage": f.Percentage,
			}
			if f.Expiry != "" {
				entry["expiry"] = f.Expiry
			}
			funds = append(funds, entry)
		}
		if err := patchJSONParam("app_state.protocolpool.continuous_funds", funds); err != nil {
			return err
		}
	}
	return nil
}

// capitalizeAccessType maps a lowercase wasm access type from the plan
// ("nobody", "everybody", "any_of_addresses", "anyofaddresses") to the
// capitalized enum form ("Nobody", "Everybody", "AnyOfAddresses") expected by
// the wasmd AccessType.UnmarshalText when reading genesis JSON.
func capitalizeAccessType(v string) string {
	switch v {
	case "nobody":
		return "Nobody"
	case "everybody":
		return "Everybody"
	case "any_of_addresses", "anyofaddresses":
		return "AnyOfAddresses"
	default:
		return v
	}
}

// patchJSONParam patches a parameter using JSON serialization.
func patchJSONParam(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal value for %s: %w", path, err)
	}
	if err := PatchGenesisParam(path, string(data)); err != nil {
		return err
	}
	uio.LogInfo("  Set %s = %s", path, string(data))
	return nil
}

// patchConsensusParams writes the CometBFT consensus parameters into
// genesis.json. Unset fields fall back to DefaultConsensusParams (Cosmos Hub
// production values). These params are fixed forever once the chain starts,
// so they must be chosen deliberately.
//
// Modern SDKs (v0.50+/CometBFT v1) store the params under the top-level
// "consensus" key with the params nested in "params" and integer fields
// serialized as strings. The layout is detected from the genesis file.
func patchConsensusParams(p ConsensusParams) error {
	d := resolveConsensusParams(p)

	// genesis.json stores max_age_duration as nanoseconds; the plan accepts a
	// Go duration string (e.g. "48h"). Normalize before writing.
	maxAgeDur, err := time.ParseDuration(d.EvidenceMaxAgeDuration)
	if err != nil {
		return fmt.Errorf("parse consensus.evidence_max_age_duration %q: %w", d.EvidenceMaxAgeDuration, err)
	}
	maxAgeDurNS := maxAgeDur.Nanoseconds()

	modern, err := consensusLayoutModern()
	if err != nil {
		return err
	}

	if modern {
		// New layout: consensus.params.* with string-encoded ints.
		paths := []struct {
			key   string
			value any
		}{
			{"consensus.params.block.max_bytes", strconv.FormatInt(d.BlockMaxBytes, 10)},
			{"consensus.params.block.max_gas", strconv.FormatInt(d.BlockMaxGas, 10)},
			{"consensus.params.evidence.max_age_num_blocks", strconv.FormatInt(d.EvidenceMaxAgeNumBlocks, 10)},
			{"consensus.params.evidence.max_age_duration", strconv.FormatInt(maxAgeDurNS, 10)},
			{"consensus.params.evidence.max_bytes", strconv.FormatInt(d.EvidenceMaxBytes, 10)},
			{"consensus.params.validator.pub_key_types", d.ValidatorPubKeyTypes},
		}
		// AuthorityParams lives only in the modern layout. When unset it is
		// left untouched so the chain keeps its default (module-keeper) value.
		if d.Authority != "" {
			paths = append(paths, struct {
				key   string
				value any
			}{"consensus.params.authority.authority", d.Authority})
		}
		for _, item := range paths {
			if err := patchJSONParam(item.key, item.value); err != nil {
				return err
			}
		}
		return nil
	}

	// Legacy layout: consensus_params.* with numeric ints.
	paths := []struct {
		key   string
		value any
	}{
		{"consensus_params.block.max_bytes", d.BlockMaxBytes},
		{"consensus_params.block.max_gas", d.BlockMaxGas},
		{"consensus_params.evidence.max_age_num_blocks", d.EvidenceMaxAgeNumBlocks},
		{"consensus_params.evidence.max_age_duration", maxAgeDurNS},
		{"consensus_params.evidence.max_bytes", d.EvidenceMaxBytes},
		{"consensus_params.validator.pub_key_types", d.ValidatorPubKeyTypes},
	}
	// time_iota_ms exists only in legacy layouts; skip if absent to avoid
	// injecting an unknown field that could break genesis validation.
	for _, item := range paths {
		if err := patchJSONParam(item.key, item.value); err != nil {
			return err
		}
	}
	return nil
}

// consensusLayoutModern reports whether the genesis uses the modern top-level
// "consensus" key (CometBFT v1) instead of the legacy "consensus_params".
func consensusLayoutModern() (bool, error) {
	genesis := filepath.Join(Home(), "config", "genesis.json")
	data, err := os.ReadFile(genesis)
	if err != nil {
		return false, fmt.Errorf("read genesis: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, fmt.Errorf("parse genesis: %w", err)
	}
	_, hasModern := doc["consensus"]
	return hasModern, nil
}
