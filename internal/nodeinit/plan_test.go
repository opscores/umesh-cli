package nodeinit

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParsePlan(t *testing.T) {
	yaml := `
chain:
  chain_id: "umesh-testnet-1"
  moniker: "umesh-genesis"
  denom: "uumesh"
  decimals: 6
  genesis_time: "2026-08-15T00:00:00Z"
  denom_uri: "https://github.com/opscores/node-umesh"
  constitution: |
    Umesh is a sovereign interchain platform built on the Cosmos SDK.
    Placeholder constitution for testing.

tokenomics:
  total_supply: "1000000000000000"
  allocations:
    - name: "foundation"
      type: "base_account"
      percentage: 20.0
      key_name: "foundation"
    - name: "team"
      type: "continuous_vesting"
      percentage: 15.0
      key_name: "team"
      vesting:
        start_time: "2026-08-15T00:00:00Z"
        end_time: "2029-08-15T00:00:00Z"
    - name: "investors"
      type: "delayed_vesting"
      percentage: 10.0
      key_name: "investors"
      vesting:
        end_time: "2028-08-15T00:00:00Z"
    - name: "ecosystem"
      type: "base_account"
      percentage: 15.0
      key_name: "ecosystem"
    - name: "airdrop"
      type: "base_account"
      percentage: 25.0
      key_name: "airdrop"
    - name: "validators"
      type: "validator_set"
      percentage: 15.0
      validators:
        - name: "validator-1"
          self_delegation: "100000000000000"
          commission_rate: "0.05"
          commission_max: "0.20"
          commission_max_change: "0.01"
          min_self_delegation: "1"
          operational_funds: "50000000000000"
          website: "https://github.com/opscores/node-umesh"
          security_contact: "security@umesh.network"
          details: "Umesh Network genesis validator"
  validation:
    max_single_allocation_percent: 25.0
    max_insider_allocation_percent: 45.0
    min_validator_count: 1
    dust_destination: "community_pool"

modules:
  staking:
    max_validators: 100
    unbonding_time: "1814400s"

  distribution:
    community_tax: "0.02"
  bank:
    default_send_enabled: true
  mint:
    inflation_max: "0.20"
    inflation_min: "0.07"
    goal_bonded: "0.67"
  gov:
    min_deposit: "1000000000"
    expedited_min_deposit: "2000000000"
    voting_period: "1209600s"
    quorum: "0.334"
    threshold: "0.50"
    veto_threshold: "0.334"
    min_initial_deposit_ratio: "0.100000000000000000"
    burn_vote_quorum: true
    burn_proposal_deposit_prevote: true
    expedited_voting_period: "86400s"
    expedited_threshold: "0.670000000000000000"
    proposal_cancel_ratio: "0.500000000000000000"
    proposal_cancel_dest: "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g"
    burn_vote_veto: true
    min_deposit_ratio: "0.010000000000000000"
    starting_proposal_id: "1"
  slashing:
    slash_fraction_double_sign: "0.05"
  wasm:
    code_upload_access: "nobody"
    instantiate_default_permission: "everybody"

soft_launch:
  enabled: true
  disable_bank_send: true
  disable_ibc_transfer: true
  allow_staking: true
  allow_gov: true
  disable_inflation: false
`

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "plan.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	plan, err := ParsePlan(path)
	if err != nil {
		t.Fatalf("parse plan: %v", err)
	}

	if plan.Chain.ChainID != "umesh-testnet-1" {
		t.Errorf("chain_id = %q, want umesh-testnet-1", plan.Chain.ChainID)
	}
	if plan.Chain.Denom != "uumesh" {
		t.Errorf("denom = %q, want uumesh", plan.Chain.Denom)
	}
	if plan.Tokenomics.TotalSupply != "1000000000000000" {
		t.Errorf("total_supply = %q, want 1000000000000000", plan.Tokenomics.TotalSupply)
	}
	if len(plan.Tokenomics.Allocations) != 6 {
		t.Errorf("allocations = %d, want 6", len(plan.Tokenomics.Allocations))
	}
	if plan.Chain.DenomURI != "https://github.com/opscores/node-umesh" {
		t.Errorf("denom_uri = %q, want https://github.com/opscores/node-umesh", plan.Chain.DenomURI)
	}
	if !strings.Contains(plan.Chain.Constitution, "Umesh") {
		t.Errorf("constitution = %q, want text mentioning Umesh", plan.Chain.Constitution)
	}
	if !plan.Modules.Gov.BurnVoteQuorum {
		t.Error("burn_vote_quorum = false, want true")
	}
	if !plan.Modules.Gov.BurnProposalDepositPrevote {
		t.Error("burn_proposal_deposit_prevote = false, want true")
	}
	// New gov params (SDK 0.50+)
	if plan.Modules.Gov.ExpeditedVotingPeriod != "86400s" {
		t.Errorf("expedited_voting_period = %q, want 86400s", plan.Modules.Gov.ExpeditedVotingPeriod)
	}
	if plan.Modules.Gov.ExpeditedThreshold != "0.670000000000000000" {
		t.Errorf("expedited_threshold = %q, want 0.670000000000000000", plan.Modules.Gov.ExpeditedThreshold)
	}
	if plan.Modules.Gov.ProposalCancelRatio != "0.500000000000000000" {
		t.Errorf("proposal_cancel_ratio = %q, want 0.500000000000000000", plan.Modules.Gov.ProposalCancelRatio)
	}
	if plan.Modules.Gov.ProposalCancelDest != "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g" {
		t.Errorf("proposal_cancel_dest = %q, want address", plan.Modules.Gov.ProposalCancelDest)
	}
	if !plan.Modules.Gov.BurnVoteVeto {
		t.Error("burn_vote_veto = false, want true")
	}
	if plan.Modules.Gov.MinDepositRatio != "0.010000000000000000" {
		t.Errorf("min_deposit_ratio = %q, want 0.010000000000000000", plan.Modules.Gov.MinDepositRatio)
	}
	if plan.Modules.Gov.StartingProposalID != "1" {
		t.Errorf("starting_proposal_id = %q, want 1", plan.Modules.Gov.StartingProposalID)
	}
	// Bank params
	if !plan.Modules.Bank.DefaultSendEnabled {
		t.Error("bank.default_send_enabled = false, want true")
	}
	// Staking params

	// Soft launch
	if plan.SoftLaunch.DisableInflation {
		t.Error("soft_launch.disable_inflation = true, want false")
	}
	if plan.Tokenomics.Allocations[5].Type != "validator_set" || len(plan.Tokenomics.Allocations[5].Validators) == 0 {
		t.Fatal("expected validator_set allocation with validators")
	}
	v := plan.Tokenomics.Allocations[5].Validators[0]
	if v.Website != "https://github.com/opscores/node-umesh" {
		t.Errorf("validator website = %q, want github url", v.Website)
	}
	if v.SecurityContact != "security@umesh.network" {
		t.Errorf("validator security_contact = %q, want security@umesh.network", v.SecurityContact)
	}
	if !strings.Contains(v.Details, "Umesh Network genesis validator") {
		t.Errorf("validator details = %q, want placeholder", v.Details)
	}
}

func TestValidateConsensusAuthority(t *testing.T) {
	tests := []struct {
		name      string
		authority string
		wantErr   bool
	}{
		{"empty authority allowed", "", false},
		{"valid umesh address", "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g", false},
		{"valid cosmos address", "cosmos10d07y265gmmuvt4z0w9aw880jnsr700j6zn9kn", false},
		{"no separator", "umesh0d07y265gmmuvt4z0w9aw880jnsr700jplz74g", true},
		{"uppercase", "umesh10d07y265GMMUVT4Z0W9AW880JNSR700JPLZ74G", true},
		{"forbidden bech32 chars", "umesh10o07y265gmmuvt4z0w9aw880jnsr700jplz74g", true},
		{"empty hrp", "1qpzry9x8gf2tvdw0s3jn54khce6mua7l", true},
		{"too long", "umesh1" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConsensusParams(&ConsensusParams{Authority: tt.authority})
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConsensusParams(authority=%q) error = %v, wantErr %v", tt.authority, err, tt.wantErr)
			}
		})
	}
}

func TestResolveConsensusParamsAuthority(t *testing.T) {
	d := resolveConsensusParams(ConsensusParams{
		Authority: "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g",
	})
	if d.Authority != "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g" {
		t.Errorf("authority = %q, want resolved value", d.Authority)
	}
	if d.BlockMaxGas != -1 {
		t.Errorf("block_max_gas = %d, want default -1", d.BlockMaxGas)
	}
}

func TestValidatePlan(t *testing.T) {
	tests := []struct {
		name    string
		plan    *Plan
		wantErr bool
		check   func(plan *Plan)
	}{
		{
			name: "valid plan",
			plan: &Plan{
				Chain: ChainConfig{
					ChainID: "umesh-1",
					Moniker: "test",
					Denom:   "uumesh",
				},
				Tokenomics: Tokenomics{
					TotalSupply: "1000000",
					Allocations: []Allocation{
						{Name: "a", Type: "base_account", Percentage: 50, KeyName: "a"},
						{Name: "b", Type: "base_account", Percentage: 30, KeyName: "b"},
						{Name: "vals", Type: "validator_set", Percentage: 20, Validators: []ValidatorConfig{
							{Name: "val1", SelfDelegation: "1000", CommissionRate: "0.1"},
						}},
					},
					Validation: Validation{
						MaxSingleAllocation:  60,
						MaxInsiderAllocation: 80,
						MinValidatorCount:    1,
						DustDestination:      "community_pool",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "total not 100%",
			plan: &Plan{
				Chain: ChainConfig{
					ChainID: "umesh-1",
					Moniker: "test",
					Denom:   "uumesh",
				},
				Tokenomics: Tokenomics{
					TotalSupply: "1000000",
					Allocations: []Allocation{
						{Name: "a", Type: "base_account", Percentage: 40, KeyName: "a"},
						{Name: "b", Type: "base_account", Percentage: 50, KeyName: "b"},
					},
					Validation: Validation{
						MaxSingleAllocation:  60,
						MaxInsiderAllocation: 80,
						MinValidatorCount:    0,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "exceeds single allocation",
			plan: &Plan{
				Chain: ChainConfig{
					ChainID: "umesh-1",
					Moniker: "test",
					Denom:   "uumesh",
				},
				Tokenomics: Tokenomics{
					TotalSupply: "1000000",
					Allocations: []Allocation{
						{Name: "a", Type: "base_account", Percentage: 30, KeyName: "a"},
						{Name: "b", Type: "base_account", Percentage: 70, KeyName: "b"},
					},
					Validation: Validation{
						MaxSingleAllocation:  60,
						MaxInsiderAllocation: 80,
						MinValidatorCount:    0,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid allocation type",
			plan: &Plan{
				Chain: ChainConfig{
					ChainID: "umesh-1",
					Moniker: "test",
					Denom:   "uumesh",
				},
				Tokenomics: Tokenomics{
					TotalSupply: "1000000",
					Allocations: []Allocation{
						{Name: "a", Type: "invalid_type", Percentage: 100, KeyName: "a"},
					},
					Validation: Validation{
						MaxSingleAllocation:  100,
						MaxInsiderAllocation: 100,
						MinValidatorCount:    0,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing chain_id",
			plan: &Plan{
				Chain: ChainConfig{
					Moniker: "test",
					Denom:   "uumesh",
				},
				Tokenomics: Tokenomics{
					TotalSupply: "1000000",
					Allocations: []Allocation{
						{Name: "a", Type: "base_account", Percentage: 100, KeyName: "a"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "vesting starts before genesis_time is clamped",
			plan: &Plan{
				Chain: ChainConfig{
					ChainID:     "umesh-1",
					Moniker:     "test",
					Denom:       "uumesh",
					GenesisTime: "2026-08-15T00:00:00Z",
				},
				Tokenomics: Tokenomics{
					TotalSupply: "1000000",
					Allocations: []Allocation{
						{Name: "team", Type: "continuous_vesting", Percentage: 30, KeyName: "team", Vesting: &VestingConfig{
							StartTime: "2026-06-15T00:00:00Z",
							EndTime:   "2029-08-15T00:00:00Z",
						}},
						{Name: "b", Type: "base_account", Percentage: 30, KeyName: "b"},
						{Name: "vals", Type: "validator_set", Percentage: 40, Validators: []ValidatorConfig{
							{Name: "val1", SelfDelegation: "1000", CommissionRate: "0.1"},
						}},
					},
					Validation: Validation{
						MaxSingleAllocation:  40,
						MaxInsiderAllocation: 80,
						MinValidatorCount:    1,
					},
				},
			},
			wantErr: false,
			check: func(plan *Plan) {
				if got := plan.Tokenomics.Allocations[0].Vesting.StartTime; got != "2026-08-15T00:00:00Z" {
					t.Errorf("vesting start_time = %q, want clamped to genesis_time", got)
				}
			},
		},
		{
			name: "vesting start after end",
			plan: &Plan{
				Chain: ChainConfig{
					ChainID:     "umesh-1",
					Moniker:     "test",
					Denom:       "uumesh",
					GenesisTime: "2026-08-15T00:00:00Z",
				},
				Tokenomics: Tokenomics{
					TotalSupply: "1000000",
					Allocations: []Allocation{
						{Name: "team", Type: "continuous_vesting", Percentage: 30, KeyName: "team", Vesting: &VestingConfig{
							StartTime: "2029-08-15T00:00:00Z",
							EndTime:   "2028-08-15T00:00:00Z",
						}},
						{Name: "b", Type: "base_account", Percentage: 30, KeyName: "b"},
						{Name: "vals", Type: "validator_set", Percentage: 40, Validators: []ValidatorConfig{
							{Name: "val1", SelfDelegation: "1000", CommissionRate: "0.1"},
						}},
					},
					Validation: Validation{
						MaxSingleAllocation:  40,
						MaxInsiderAllocation: 80,
						MinValidatorCount:    1,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "vesting starts at genesis_time",
			plan: &Plan{
				Chain: ChainConfig{
					ChainID:     "umesh-1",
					Moniker:     "test",
					Denom:       "uumesh",
					GenesisTime: "2026-08-15T00:00:00Z",
				},
				Tokenomics: Tokenomics{
					TotalSupply: "1000000",
					Allocations: []Allocation{
						{Name: "team", Type: "continuous_vesting", Percentage: 30, KeyName: "team", Vesting: &VestingConfig{
							StartTime: "2026-08-15T00:00:00Z",
							EndTime:   "2029-08-15T00:00:00Z",
						}},
						{Name: "b", Type: "base_account", Percentage: 30, KeyName: "b"},
						{Name: "vals", Type: "validator_set", Percentage: 40, Validators: []ValidatorConfig{
							{Name: "val1", SelfDelegation: "1000", CommissionRate: "0.1"},
						}},
					},
					Validation: Validation{
						MaxSingleAllocation:  40,
						MaxInsiderAllocation: 80,
						MinValidatorCount:    1,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlan(tt.plan)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlan() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(tt.plan)
			}
		})
	}
}

func TestPercentageToAmount(t *testing.T) {
	totalSupply := big.NewInt(1000000000000000) // 1B
	tests := []struct {
		percentage float64
		want       string
	}{
		{20.0, "200000000000000"},
		{15.0, "150000000000000"},
		{10.0, "100000000000000"},
		{33.33, "333299999999999"}, // Floating point precision
		{0.01, "100000000000"},
	}

	for _, tt := range tests {
		got := percentageToAmount(tt.percentage, totalSupply)
		if got.String() != tt.want {
			t.Errorf("percentageToAmount(%f) = %s, want %s", tt.percentage, got.String(), tt.want)
		}
	}
}

func TestPrintPlanReport(t *testing.T) {
	plan := &Plan{
		Chain: ChainConfig{
			ChainID: "umesh-testnet-1",
			Moniker: "umesh-genesis",
			Denom:   "uumesh",
		},
		Tokenomics: Tokenomics{
			TotalSupply: "1000000000000000",
			Allocations: []Allocation{
				{Name: "foundation", Type: "base_account", Percentage: 20, KeyName: "foundation"},
				{Name: "team", Type: "continuous_vesting", Percentage: 15, KeyName: "team", Vesting: &VestingConfig{
					StartTime: "2026-08-15T00:00:00Z",
					EndTime:   "2029-08-15T00:00:00Z",
				}},
				{Name: "investors", Type: "delayed_vesting", Percentage: 10, KeyName: "investors", Vesting: &VestingConfig{
					EndTime: "2028-08-15T00:00:00Z",
				}},
			},
		},
	}

	SetHome(t.TempDir())

	err := PrintPlanReport(plan, "table")
	if err != nil {
		t.Fatalf("PrintPlanReport() error: %v", err)
	}
}

func TestPrintPlanReportJSON(t *testing.T) {
	plan := &Plan{
		Chain: ChainConfig{
			ChainID: "umesh-testnet-1",
			Moniker: "umesh-genesis",
			Denom:   "uumesh",
		},
		Tokenomics: Tokenomics{
			TotalSupply: "1000000000000000",
			Allocations: []Allocation{
				{Name: "foundation", Type: "base_account", Percentage: 20, KeyName: "foundation"},
				{Name: "team", Type: "continuous_vesting", Percentage: 15, KeyName: "team"},
			},
		},
	}

	SetHome(t.TempDir())

	err := PrintPlanReport(plan, "json")
	if err != nil {
		t.Fatalf("PrintPlanReport() error: %v", err)
	}
}

func TestValidateAllocationType(t *testing.T) {
	valid := []string{"base_account", "delayed_vesting", "continuous_vesting", "validator_set"}
	for _, typ := range valid {
		if err := validateAllocationType(typ); err != nil {
			t.Errorf("validateAllocationType(%q) = %v, want nil", typ, err)
		}
	}

	invalid := []string{"unknown", "", "vesting", "base", "clawback_vesting"}
	for _, typ := range invalid {
		if err := validateAllocationType(typ); err == nil {
			t.Errorf("validateAllocationType(%q) = nil, want error", typ)
		}
	}
}

func TestIsInsider(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"team", true},
		{"investors", true},
		{"foundation", true},
		{"Team", true},
		{"Investors", true},
		{"ecosystem", false},
		{"airdrop", false},
		{"validators", false},
	}

	for _, tt := range tests {
		if got := IsInsider(tt.name); got != tt.expected {
			t.Errorf("IsInsider(%q) = %v, want %v", tt.name, got, tt.expected)
		}
	}
}

func TestParseUnixTime(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"2026-08-15T00:00:00Z", false},
		{"2029-08-15T00:00:00Z", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		got, err := parseUnixTime(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseUnixTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if !tt.wantErr && got <= 0 {
			t.Errorf("parseUnixTime(%q) = %d, want > 0", tt.input, got)
		}
	}
}

func TestResolveGenesisTime(t *testing.T) {
	// "now" and "" resolve to a valid RFC3339 timestamp in the past/present.
	for _, input := range []string{"now", ""} {
		got := resolveGenesisTime(input)
		ts, err := parseGenesisTime(got)
		if err != nil {
			t.Errorf("resolveGenesisTime(%q) = %q, not RFC3339: %v", input, got, err)
		}
		if ts.After(time.Now().Add(time.Minute)) {
			t.Errorf("resolveGenesisTime(%q) = %q, should be ~now", input, got)
		}
	}
	// A fixed timestamp passes through unchanged.
	got := resolveGenesisTime("2026-08-15T00:00:00Z")
	if got != "2026-08-15T00:00:00Z" {
		t.Errorf("resolveGenesisTime(fixed) = %q, want pass-through", got)
	}
	// Vesting validation accepts a "now" genesis_time.
	if err := validateTokenomics(&Tokenomics{
		TotalSupply: "1000000000000000",
		Validation:  Validation{MaxSingleAllocation: 100.0, MaxInsiderAllocation: 100.0, MinValidatorCount: 0},
		Allocations: []Allocation{
			{
				Name:       "team",
				Type:       "continuous_vesting",
				Percentage: 25.0,
				KeyName:    "team",
				Vesting:    &VestingConfig{
					StartTime: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
					EndTime:   time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339),
				},
			},
			{
				Name:       "foundation",
				Type:       "base_account",
				Percentage: 25.0,
				KeyName:    "foundation",
			},
			{
				Name:       "ecosystem",
				Type:       "base_account",
				Percentage: 25.0,
				KeyName:    "ecosystem",
			},
			{
				Name:       "validators",
				Type:       "validator_set",
				Percentage: 25.0,
				KeyName:    "validators",
				Validators: []ValidatorConfig{{
					Name:           "validator",
					SelfDelegation: "1000000000000",
					CommissionRate: "0.05",
				}},
			},
		},
	}, "now"); err != nil {
		t.Errorf("validateTokenomics with genesis_time=now error: %v", err)
	}
}

func TestHandleDustFoundation(t *testing.T) {
	SetHome(t.TempDir())

	genesis := `{
  "app_state": {
    "bank": {
      "balances": [
        {"address": "umesh1foundation", "coins": [{"denom": "uumesh", "amount": "500"}]}
      ]
    }
  }
}`
	path := filepath.Join(Home(), "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(genesis), 0o644); err != nil {
		t.Fatalf("write genesis: %v", err)
	}

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Tokenomics: Tokenomics{
			TotalSupply: "1000",
			Allocations: []Allocation{
				{Name: "foundation", Type: "base_account", Percentage: 50},
			},
			Validation: Validation{DustDestination: "foundation"},
		},
	}
	resolved := []ResolvedAllocation{
		{Name: "foundation", Address: "umesh1foundation", Amount: big.NewInt(500)},
	}

	if err := handleDust(plan, resolved); err != nil {
		t.Fatalf("handleDust() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read genesis: %v", err)
	}
	var gen map[string]any
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatalf("parse genesis: %v", err)
	}

	appState := gen["app_state"].(map[string]any)
	bank := appState["bank"].(map[string]any)
	balances := bank["balances"].([]any)
	entry := balances[0].(map[string]any)
	if got := fmt.Sprintf("%v", entry["address"]); got != "umesh1foundation" {
		t.Fatalf("address = %q, want umesh1foundation", got)
	}
	coins := entry["coins"].([]any)
	coin := coins[0].(map[string]any)
	if got := fmt.Sprintf("%v", coin["amount"]); got != "1000" {
		t.Errorf("foundation balance = %s, want 1000 (500 + 500 dust)", got)
	}
}

func TestHandleDustFoundationMissingFallsBack(t *testing.T) {
	SetHome(t.TempDir())

	genesis := `{
  "app_state": {
    "bank": {
      "balances": [
        {"address": "umesh1other", "coins": [{"denom": "uumesh", "amount": "500"}]}
      ]
    }
  }
}`
	path := filepath.Join(Home(), "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(genesis), 0o644); err != nil {
		t.Fatalf("write genesis: %v", err)
	}

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Tokenomics: Tokenomics{
			TotalSupply: "1000",
			Allocations: []Allocation{
				{Name: "team", Type: "base_account", Percentage: 50},
			},
			Validation: Validation{DustDestination: "foundation"},
		},
	}
	resolved := []ResolvedAllocation{
		{Name: "team", Address: "umesh1other", Amount: big.NewInt(500)},
	}

	if err := handleDust(plan, resolved); err != nil {
		t.Fatalf("handleDust() error: %v", err)
	}

	// Fallback writes to community pool; foundation balance stays untouched.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read genesis: %v", err)
	}
	var gen map[string]any
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatalf("parse genesis: %v", err)
	}
	appState := gen["app_state"].(map[string]any)
	bank := appState["bank"].(map[string]any)
	balances := bank["balances"].([]any)
	entry := balances[0].(map[string]any)
	coins := entry["coins"].([]any)
	coin := coins[0].(map[string]any)
	if got := fmt.Sprintf("%v", coin["amount"]); got != "500" {
		t.Errorf("balance changed to %s, want unchanged 500", got)
	}
}
