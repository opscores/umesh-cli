package nodeinit

import (
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Plan is the top-level genesis plan document.
type Plan struct {
	Chain      ChainConfig `yaml:"chain"`
	Tokenomics Tokenomics  `yaml:"tokenomics"`
	Modules    Modules     `yaml:"modules"`
	SoftLaunch SoftLaunch  `yaml:"soft_launch"`
}

// ChainConfig describes the chain identity.
type ChainConfig struct {
	ChainID     string          `yaml:"chain_id"`
	Moniker     string          `yaml:"moniker"`
	Denom       string          `yaml:"denom"`
	Decimals    int             `yaml:"decimals"`
	GenesisTime string          `yaml:"genesis_time"`
	Consensus   ConsensusParams `yaml:"consensus"`
	// DenomURI is an optional URI to a document with additional information
	// about the denom (off-chain), stored in bank denom_metadata.uri.
	DenomURI string `yaml:"denom_uri"`
	// Constitution is an immutable on-chain document describing the purpose
	// and ideals of the network, stored in gov.constitution.
	Constitution string `yaml:"constitution"`
}

// ConsensusParams describes the CometBFT consensus parameters written into
// genesis.json. They are fixed forever once the chain starts, so the defaults
// mirror the production values used by Cosmos Hub (cosmoshub-4). Any field
// left at its zero value in the plan falls back to these hardened defaults.
type ConsensusParams struct {
	BlockMaxBytes           int64    `yaml:"block_max_bytes"`
	BlockMaxGas             int64    `yaml:"block_max_gas"`
	TimeIotaMs              int64    `yaml:"time_iota_ms"`
	EvidenceMaxAgeNumBlocks int64    `yaml:"evidence_max_age_num_blocks"`
	EvidenceMaxAgeDuration  string   `yaml:"evidence_max_age_duration"`
	EvidenceMaxBytes        int64    `yaml:"evidence_max_bytes"`
	ValidatorPubKeyTypes    []string `yaml:"validator_pub_key_types"`
	// Authority is the address allowed to update consensus params via
	// MsgUpdateParams (SDK 0.54 x/consensus AuthorityParams). When empty, the
	// module-level keeper authority (gov module address) is used as fallback.
	Authority string `yaml:"authority"`
}

// Tokenomics describes the token distribution.
type Tokenomics struct {
	TotalSupply string       `yaml:"total_supply"`
	Allocations []Allocation `yaml:"allocations"`
	Validation  Validation   `yaml:"validation"`
}

// Allocation is a single token allocation entry.
type Allocation struct {
	Name       string            `yaml:"name"`
	Type       string            `yaml:"type"`
	Percentage float64           `yaml:"percentage"`
	KeyName    string            `yaml:"key_name"`
	Address    string            `yaml:"address"`
	Mnemonic   string            `yaml:"mnemonic"`
	Vesting    *VestingConfig    `yaml:"vesting"`
	Validators []ValidatorConfig `yaml:"validators"`
	ModuleName string            `yaml:"module_name"`
}

// VestingConfig describes vesting parameters.
type VestingConfig struct {
	StartTime       string `yaml:"start_time"`
	EndTime         string `yaml:"end_time"`
	CliffDuration   string `yaml:"cliff_duration"`
	VestingDuration string `yaml:"vesting_duration"`
}

// ValidatorConfig describes a genesis validator.
type ValidatorConfig struct {
	Name                string `yaml:"name"`
	SelfDelegation      string `yaml:"self_delegation"`
	CommissionRate      string `yaml:"commission_rate"`
	CommissionMax       string `yaml:"commission_max"`
	CommissionMaxChange string `yaml:"commission_max_change"`
	MinSelfDelegation   string `yaml:"min_self_delegation"`
	OperationalFunds    string `yaml:"operational_funds"`
	ExternalAddress     string `yaml:"external_address"`
	// Identity is a 16-char Keybase ID linking the validator to a profile/avatar.
	Identity string `yaml:"identity"`
	// Website is the validator's public website URL.
	Website string `yaml:"website"`
	// SecurityContact is an email for security contact of the validator.
	SecurityContact string `yaml:"security_contact"`
	// Details is a free-form description of the validator.
	Details string `yaml:"details"`
}

// Validation describes allocation validation rules.
type Validation struct {
	MaxSingleAllocation  float64 `yaml:"max_single_allocation_percent"`
	MaxInsiderAllocation float64 `yaml:"max_insider_allocation_percent"`
	MinValidatorCount    int     `yaml:"min_validator_count"`
	DustDestination      string  `yaml:"dust_destination"`
}

// Modules describes module parameters.
type Modules struct {
	Staking      StakingParams      `yaml:"staking"`
	Distribution DistributionParams `yaml:"distribution"`
	Mint         MintParams         `yaml:"mint"`
	Gov          GovParams          `yaml:"gov"`
	Slashing     SlashingParams     `yaml:"slashing"`
	Wasm         WasmParams         `yaml:"wasm"`
	Epochs       EpochsParams       `yaml:"epochs"`
	ProtocolPool ProtocolPoolParams `yaml:"protocolpool"`
	Bank         BankParams         `yaml:"bank"`
}

// BankParams describes bank module parameters.
type BankParams struct {
	// DefaultSendEnabled is the default send_enabled state for new denoms.
	// SDK default: true. Set to false to disable transfers by default.
	DefaultSendEnabled bool `yaml:"default_send_enabled"`
}

// StakingParams describes staking module parameters.
type StakingParams struct {
	MaxValidators     int64  `yaml:"max_validators"`
	MaxEntries        int64  `yaml:"max_entries"`
	HistoricalEntries int64  `yaml:"historical_entries"`
	UnbondingTime     string `yaml:"unbonding_time"`
	BondDenom         string `yaml:"bond_denom"`
	// MinCommissionRate is the floor on the commission rate a validator can
	// set. Empty leaves the default (0).
	MinCommissionRate string `yaml:"min_commission_rate"`
}

// DistributionParams describes distribution module parameters.
type DistributionParams struct {
	CommunityTax        string `yaml:"community_tax"`
	WithdrawAddrEnabled bool   `yaml:"withdraw_addr_enabled"`
}

// MintParams describes mint module parameters.
type MintParams struct {
	MintDenom           string `yaml:"mint_denom"`
	InflationRateChange string `yaml:"inflation_rate_change"`
	InflationMax        string `yaml:"inflation_max"`
	InflationMin        string `yaml:"inflation_min"`
	GoalBonded          string `yaml:"goal_bonded"`
	BlocksPerYear       string `yaml:"blocks_per_year"`
	// MaxSupply is the hard cap on total supply; minting stops when reached.
	// A value of "0" (the SDK default) means no cap / unlimited inflation.
	MaxSupply string `yaml:"max_supply"`
}

// GovParams describes governance module parameters.
type GovParams struct {
	MinDeposit            string `yaml:"min_deposit"`
	MaxDepositPeriod      string `yaml:"max_deposit_period"`
	VotingPeriod          string `yaml:"voting_period"`
	Quorum                string `yaml:"quorum"`
	Threshold             string `yaml:"threshold"`
	VetoThreshold         string `yaml:"veto_threshold"`
	MinInitialDepositRatio string `yaml:"min_initial_deposit_ratio"`
	// ExpeditedMinDeposit is the deposit required for expedited proposals. Must
	// be strictly greater than min_deposit (SDK invariant).
	ExpeditedMinDeposit string `yaml:"expedited_min_deposit"`
	// BurnVoteQuorum burns the proposal deposit when the proposal does not
	// meet quorum (anti-spam).
	BurnVoteQuorum bool `yaml:"burn_vote_quorum"`
	// BurnProposalDepositPrevote burns the proposal deposit when the proposal
	// does not enter the voting period (anti-spam).
	BurnProposalDepositPrevote bool `yaml:"burn_proposal_deposit_prevote"`

	// ExpeditedVotingPeriod is the voting period for expedited proposals.
	// SDK default: "86400s" (24h). Set to "0s" to disable expedited proposals.
	ExpeditedVotingPeriod string `yaml:"expedited_voting_period"`
	// ExpeditedThreshold is the pass threshold for expedited proposals.
	// SDK default: "0.670000000000000000" (67%). Must be > regular threshold.
	ExpeditedThreshold string `yaml:"expedited_threshold"`
	// ProposalCancelRatio is the fraction of deposit burned when a proposal
	// is cancelled. SDK default: "0.500000000000000000" (50%).
	ProposalCancelRatio string `yaml:"proposal_cancel_ratio"`
	// ProposalCancelDest is the address that receives canceled proposal funds.
	// Empty string (default) means funds are burned.
	ProposalCancelDest string `yaml:"proposal_cancel_dest"`
	// BurnVoteVeto burns the deposit when NO_WITH_VETO wins.
	// SDK default: false.
	BurnVoteVeto bool `yaml:"burn_vote_veto"`
	// MinDepositRatio is the minimum ratio of initial deposit to min_deposit.
	// SDK default: "0.010000000000000000" (1%).
	MinDepositRatio string `yaml:"min_deposit_ratio"`
	// StartingProposalID is the first governance proposal ID.
	// SDK default: "1". Useful for forks.
	StartingProposalID string `yaml:"starting_proposal_id"`
}

// SlashingParams describes slashing module parameters.
type SlashingParams struct {
	SignedBlocksWindow      string `yaml:"signed_blocks_window"`
	MinSignedPerWindow      string `yaml:"min_signed_per_window"`
	DowntimeJailDuration    string `yaml:"downtime_jail_duration"`
	SlashFractionDoubleSign string `yaml:"slash_fraction_double_sign"`
	SlashFractionDowntime   string `yaml:"slash_fraction_downtime"`
}

// WasmParams describes wasm module parameters.
type WasmParams struct {
	CodeUploadAccess             string `yaml:"code_upload_access"`
	InstantiateDefaultPermission string `yaml:"instantiate_default_permission"`
}

// EpochsParams describes x/epochs genesis configuration. Only the operator-set
// fields are exposed; the SDK initializes the runtime state (current_epoch,
// epoch_counting_started, etc.) during InitGenesis.
type EpochsParams struct {
	Epochs []EpochInfo `yaml:"epochs"`
}

// EpochInfo describes a single epoch timer of the x/epochs module.
type EpochInfo struct {
	// Identifier is a unique reference to this particular timer.
	Identifier string `yaml:"identifier"`
	// StartTime is when the timer first ticks. Empty defaults to genesis time.
	StartTime string `yaml:"start_time"`
	// Duration is the interval between ticks, a Go duration string (e.g. "86400s").
	// Must be non-zero and greater than the chain's expected block time.
	Duration string `yaml:"duration"`
}

// ProtocolPoolParams describes x/protocolpool genesis configuration. It is kept
// separate from x/distribution: the community pool (distribution.fee_pool) and
// protocolpool funds are independent state.
type ProtocolPoolParams struct {
	EnabledDistributionDenoms []string         `yaml:"enabled_distribution_denoms"`
	DistributionFrequency     uint64           `yaml:"distribution_frequency"`
	ContinuousFunds           []ContinuousFund `yaml:"continuous_funds"`
}

// ContinuousFund describes a protocolpool continuous fund entry.
type ContinuousFund struct {
	// Recipient is the account address receiving funds.
	Recipient string `yaml:"recipient"`
	// Percentage of funds allocated from the community pool, a decimal in (0, 1].
	Percentage string `yaml:"percentage"`
	// Expiry is an optional RFC3339 time after which the fund is removed.
	Expiry string `yaml:"expiry"`
}

// SoftLaunch describes soft launch configuration.
type SoftLaunch struct {
	Enabled              bool  `yaml:"enabled"`
	DisableBankSend      bool  `yaml:"disable_bank_send"`
	DisableIBCTransfer   bool  `yaml:"disable_ibc_transfer"`
	AllowStaking         *bool `yaml:"allow_staking"`
	AllowGov             *bool `yaml:"allow_gov"`
	AllowWasmInstantiate *bool `yaml:"allow_wasm_instantiate"`
	// DisableInflation sets mint inflation to zero for the soft launch period.
	// The chain still produces blocks but no new tokens are minted.
	DisableInflation bool `yaml:"disable_inflation"`
}

// DenomMetadata describes denom metadata for bank module.
type DenomMetadata struct {
	Description string       `yaml:"description"`
	DenomUnits  []DenomUnit  `yaml:"denom_units"`
}

// DenomUnit describes a single denom unit.
type DenomUnit struct {
	Denom    string   `yaml:"denom"`
	Exponent uint32   `yaml:"exponent"`
	Aliases  []string `yaml:"aliases"`
}

// ResolvedAllocation is an allocation with resolved address and amount.
type ResolvedAllocation struct {
	Name       string
	Type       string
	Percentage float64
	Address    string
	Amount     *big.Int
	Vesting    *VestingConfig
	Validators []ValidatorConfig
}

// validateAllocationType checks if allocation type is valid.
func validateAllocationType(t string) error {
	switch t {
	case "base_account", "delayed_vesting", "continuous_vesting",
		"validator_set", "module_account":
		return nil
	case "clawback_vesting":
		// ClawbackVestingAccount is not part of vanilla Cosmos SDK (the type
		// /cosmos.vesting.v1beta1.ClawbackVestingAccount only exists in the
		// Agoric fork's separate x/vesting module). Emitting it into genesis
		// makes app_state.auth.accounts undecodable at InitGenesis, so it is
		// rejected here rather than producing a chain that refuses to start.
		return fmt.Errorf("clawback_vesting is not supported on vanilla Cosmos SDK (no ClawbackVestingAccount); use delayed_vesting or continuous_vesting instead")
	default:
		return fmt.Errorf("unknown allocation type: %q", t)
	}
}

// percentageToAmount converts a percentage of total supply to amount.
func percentageToAmount(percentage float64, totalSupply *big.Int) *big.Int {
	// amount = percentage * totalSupply / 100
	amount := new(big.Float).Mul(big.NewFloat(percentage), new(big.Float).SetInt(totalSupply))
	amount.Quo(amount, big.NewFloat(100))
	result, _ := amount.Int(nil)
	return result
}

// parseGenesisTime parses RFC3339 genesis time.
func parseGenesisTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// resolveGenesisTime resolves the genesis_time value from a plan into an
// RFC3339 timestamp. The special values "now" or "" mean "start producing
// blocks as soon as the node comes up" and are resolved to the current UTC
// time. Any other value must already be a valid RFC3339 timestamp.
func resolveGenesisTime(s string) string {
	switch {
	case s == "":
		return time.Now().UTC().Format(time.RFC3339)
	case strings.EqualFold(s, "now"):
		return time.Now().UTC().Format(time.RFC3339)
	default:
		return s
	}
}

// DefaultValidation returns default validation rules.
func DefaultValidation() Validation {
	return Validation{
		MaxSingleAllocation:  25.0,
		MaxInsiderAllocation: 45.0,
		MinValidatorCount:    1,
		DustDestination:      "community_pool",
	}
}

// DefaultConsensusParams returns production-hardened consensus parameters
// matching the values used by Cosmos Hub (cosmoshub-4). These are applied
// whenever the plan leaves a consensus field unset.
func DefaultConsensusParams() ConsensusParams {
	return ConsensusParams{
		BlockMaxBytes:           22020096,                 // 21 MiB
		BlockMaxGas:             -1,                       // unlimited
		TimeIotaMs:              1000,
		EvidenceMaxAgeNumBlocks: 100000,
		EvidenceMaxAgeDuration:  "48h",                    // Cosmos Hub uses 48h
		EvidenceMaxBytes:        1048576,                  // 1 MiB
		ValidatorPubKeyTypes:    []string{"ed25519"},
	}
}

// resolveConsensusParams merges user-supplied consensus fields over the
// production defaults. Zero-value fields are treated as "not set".
func resolveConsensusParams(p ConsensusParams) ConsensusParams {
	d := DefaultConsensusParams()
	if p.BlockMaxBytes != 0 {
		d.BlockMaxBytes = p.BlockMaxBytes
	}
	if p.BlockMaxGas != 0 {
		d.BlockMaxGas = p.BlockMaxGas
	}
	if p.TimeIotaMs != 0 {
		d.TimeIotaMs = p.TimeIotaMs
	}
	if p.EvidenceMaxAgeNumBlocks != 0 {
		d.EvidenceMaxAgeNumBlocks = p.EvidenceMaxAgeNumBlocks
	}
	if p.EvidenceMaxAgeDuration != "" {
		d.EvidenceMaxAgeDuration = p.EvidenceMaxAgeDuration
	}
	if p.EvidenceMaxBytes != 0 {
		d.EvidenceMaxBytes = p.EvidenceMaxBytes
	}
	if len(p.ValidatorPubKeyTypes) > 0 {
		d.ValidatorPubKeyTypes = p.ValidatorPubKeyTypes
	}
	if p.Authority != "" {
		d.Authority = p.Authority
	}
	return d
}

// resolveTimeIotaPtr returns time_iota_ms as a pointer when the plan sets it
// explicitly (it is only written by legacy CometBFT layouts), otherwise nil.
func resolveTimeIotaPtr(p ConsensusParams, def ConsensusParams) *int64 {
	if p.TimeIotaMs == 0 {
		return nil
	}
	v := def.TimeIotaMs
	return &v
}

// IsInsider checks if allocation name is an insider type.
func IsInsider(name string) bool {
	lower := strings.ToLower(name)
	return lower == "team" || lower == "investors" || lower == "foundation"
}
