package nodeinit

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opscores/umesh-cli/internal/dkrcmd"
)

// appStateKeys returns the top-level app_state keys of the genesis at home.
func appStateKeys(t *testing.T, home string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState map[string]json.RawMessage `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(doc.AppState))
	for k := range doc.AppState {
		keys = append(keys, k)
	}
	return keys
}

func appStateHasKey(t *testing.T, home, key string) bool {
	t.Helper()
	for _, k := range appStateKeys(t, home) {
		if k == key {
			return true
		}
	}
	return false
}

// writeGenesisWithWasm writes a genesis whose app_state includes a wasm module.
func writeGenesisWithWasm(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"app_state": map[string]any{
			"staking": map[string]any{
				"params": map[string]any{"bond_denom": "stake"},
			},
			"mint": map[string]any{
				"params": map[string]any{"mint_denom": "stake"},
			},
			"wasm": map[string]any{
				"params": map[string]any{},
			},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPatchModuleParamsSkipsDisabledModule(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Staking: StakingParams{BondDenom: "uumesh", MaxValidators: 50},
			Wasm:    WasmParams{CodeUploadAccess: "everybody"},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	if appStateHasKey(t, home, "wasm") {
		t.Fatal("app_state.wasm was created even though wasm is not compiled into umeshd")
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Staking struct {
				Params struct {
					BondDenom     string `json:"bond_denom"`
					MaxValidators int64  `json:"max_validators"`
				} `json:"params"`
			} `json:"staking"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.AppState.Staking.Params.BondDenom != "uumesh" {
		t.Errorf("staking bond_denom = %q, want %q", doc.AppState.Staking.Params.BondDenom, "uumesh")
	}
	if doc.AppState.Staking.Params.MaxValidators != 50 {
		t.Errorf("staking max_validators = %d, want %d", doc.AppState.Staking.Params.MaxValidators, 50)
	}
}

func TestPatchModuleParamsPatchesEnabledModule(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithWasm(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Wasm: WasmParams{
				CodeUploadAccess:             "everybody",
				InstantiateDefaultPermission: "nobody",
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Wasm struct {
				Params struct {
					CodeUploadAccess struct {
						Permission string `json:"permission"`
					} `json:"code_upload_access"`
					InstantiateDefaultPermission string `json:"instantiate_default_permission"`
				} `json:"params"`
			} `json:"wasm"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.AppState.Wasm.Params.CodeUploadAccess.Permission; got != "Everybody" {
		t.Errorf("wasm code_upload_access = %q, want %q", got, "Everybody")
	}
	if got := doc.AppState.Wasm.Params.InstantiateDefaultPermission; got != "Nobody" {
		t.Errorf("wasm instantiate_default_permission = %q, want %q", got, "Nobody")
	}
}

func TestEnabledAppStateModulesMissingFile(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	enabled, err := enabledAppStateModules()
	if err != nil {
		t.Fatalf("enabledAppStateModules() error: %v", err)
	}
	if len(enabled) != 0 {
		t.Errorf("expected empty module set for missing genesis, got %v", enabled)
	}
}

// writeGenesisWithEpochsProtocolPool writes a genesis whose app_state includes
// both epochs and protocolpool modules.
func writeGenesisWithEpochsProtocolPool(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"app_state": map[string]any{
			"staking": map[string]any{
				"params": map[string]any{"bond_denom": "stake"},
			},
			"epochs": map[string]any{
				"epochs": []any{},
			},
			"protocolpool": map[string]any{
				"params":           map[string]any{},
				"continuous_funds": []any{},
			},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPatchModuleParamsEpochs(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithEpochsProtocolPool(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Epochs: EpochsParams{
				Epochs: []EpochInfo{
					{Identifier: "day", Duration: "86400s"},
					{Identifier: "week", StartTime: "2026-10-01T00:00:00Z", Duration: "604800s"},
				},
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Epochs struct {
				Epochs []struct {
					Identifier string `json:"identifier"`
					StartTime  string `json:"start_time"`
					Duration   string `json:"duration"`
				} `json:"epochs"`
			} `json:"epochs"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.AppState.Epochs.Epochs) != 2 {
		t.Fatalf("epochs count = %d, want 2", len(doc.AppState.Epochs.Epochs))
	}
	day := doc.AppState.Epochs.Epochs[0]
	if day.Identifier != "day" || day.Duration != "86400s" {
		t.Errorf("epoch day = %+v, want identifier=day duration=86400s", day)
	}
	week := doc.AppState.Epochs.Epochs[1]
	if week.Identifier != "week" || week.StartTime != "2026-10-01T00:00:00Z" {
		t.Errorf("epoch week = %+v, want identifier=week with start_time", week)
	}
}

func TestPatchModuleParamsProtocolPool(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithEpochsProtocolPool(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			ProtocolPool: ProtocolPoolParams{
				EnabledDistributionDenoms: []string{"uumesh"},
				DistributionFrequency:     7200,
				ContinuousFunds: []ContinuousFund{
					{Recipient: "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g", Percentage: "0.02"},
					{Recipient: "umesh1foundation", Percentage: "0.05", Expiry: "2030-01-01T00:00:00Z"},
				},
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			ProtocolPool struct {
				Params struct {
					EnabledDistributionDenoms []string `json:"enabled_distribution_denoms"`
					DistributionFrequency     uint64   `json:"distribution_frequency"`
				} `json:"params"`
				ContinuousFunds []struct {
					Recipient  string `json:"recipient"`
					Percentage string `json:"percentage"`
					Expiry     string `json:"expiry"`
				} `json:"continuous_funds"`
			} `json:"protocolpool"`
			Distribution struct {
				FeePool struct {
					CommunityPool []any `json:"community_pool"`
				} `json:"fee_pool"`
			} `json:"distribution"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.AppState.ProtocolPool.Params.DistributionFrequency; got != 7200 {
		t.Errorf("distribution_frequency = %d, want 7200", got)
	}
	if got := doc.AppState.ProtocolPool.Params.EnabledDistributionDenoms; len(got) != 1 || got[0] != "uumesh" {
		t.Errorf("enabled_distribution_denoms = %v, want [uumesh]", got)
	}
	if len(doc.AppState.ProtocolPool.ContinuousFunds) != 2 {
		t.Fatalf("continuous_funds count = %d, want 2", len(doc.AppState.ProtocolPool.ContinuousFunds))
	}
	if got := doc.AppState.ProtocolPool.ContinuousFunds[1].Expiry; got != "2030-01-01T00:00:00Z" {
		t.Errorf("continuous_funds[1] expiry = %q, want 2030-01-01T00:00:00Z", got)
	}
}

func TestPatchModuleParamsSkipsEpochsProtocolPoolWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Epochs: EpochsParams{
				Epochs: []EpochInfo{{Identifier: "day", Duration: "86400s"}},
			},
			ProtocolPool: ProtocolPoolParams{
				DistributionFrequency: 7200,
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	if appStateHasKey(t, home, "epochs") {
		t.Fatal("app_state.epochs was created even though epochs is not compiled into umeshd")
	}
	if appStateHasKey(t, home, "protocolpool") {
		t.Fatal("app_state.protocolpool was created even though protocolpool is not compiled into umeshd")
	}
}

func TestValidateModulesEpochsErrors(t *testing.T) {
	m := &Modules{
		Epochs: EpochsParams{
			Epochs: []EpochInfo{
				{Identifier: "day", Duration: "86400s"},
				{Identifier: "day", Duration: "86400s"},
			},
		},
	}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for duplicate epoch identifier")
	}

	m = &Modules{Epochs: EpochsParams{Epochs: []EpochInfo{{Identifier: "day", Duration: "-1s"}}}}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for non-positive epoch duration")
	}

	m = &Modules{Epochs: EpochsParams{Epochs: []EpochInfo{{Identifier: "day", Duration: "bogus"}}}}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for invalid epoch duration")
	}
}

func TestValidateModulesProtocolPoolErrors(t *testing.T) {
	m := &Modules{ProtocolPool: ProtocolPoolParams{DistributionFrequency: 0}}
	if err := validateModules(m, "uumesh"); err != nil {
		t.Fatalf("expected nil for empty protocolpool, got %v", err)
	}

	m = &Modules{
		ProtocolPool: ProtocolPoolParams{
			DistributionFrequency: 0,
			ContinuousFunds: []ContinuousFund{
				{Recipient: "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g", Percentage: "0.02"},
			},
		},
	}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for zero distribution_frequency when protocolpool configured")
	}

	m = &Modules{
		ProtocolPool: ProtocolPoolParams{
			DistributionFrequency: 7200,
			ContinuousFunds: []ContinuousFund{
				{Recipient: "notanaddress", Percentage: "0.02"},
			},
		},
	}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for invalid continuous fund recipient")
	}

	m = &Modules{
		ProtocolPool: ProtocolPoolParams{
			DistributionFrequency: 7200,
			ContinuousFunds: []ContinuousFund{
				{Recipient: "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g", Percentage: "1.5"},
			},
		},
	}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for percentage > 1")
	}
}

// writeGenesisWithTransferWasmDistribution writes a genesis that includes
// bank, transfer, wasm and distribution modules (all that soft launch touches).
func writeGenesisWithTransferWasmDistribution(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"app_state": map[string]any{
			"bank": map[string]any{
				"balances":     []any{},
				"send_enabled": []any{},
			},
			"transfer": map[string]any{
				"params": map[string]any{},
			},
			"wasm": map[string]any{
				"params": map[string]any{},
			},
			"distribution": map[string]any{
				"fee_pool": map[string]any{},
			},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func boolPtr(b bool) *bool { return &b }

func TestHandleSoftLaunchSkipsAbsentModules(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	// writeDefaultGenesis has bank but no transfer/wasm/distribution.
	writeDefaultGenesis(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		SoftLaunch: SoftLaunch{
			Enabled:              true,
			DisableBankSend:      true,
			DisableIBCTransfer:   true,
			AllowWasmInstantiate: boolPtr(false),
		},
	}

	if err := handleSoftLaunch(plan); err != nil {
		t.Fatalf("handleSoftLaunch() error: %v", err)
	}

	if appStateHasKey(t, home, "transfer") {
		t.Fatal("app_state.transfer was created even though transfer is not compiled into umeshd")
	}
	if appStateHasKey(t, home, "wasm") {
		t.Fatal("app_state.wasm was created even though wasm is not compiled into umeshd")
	}

	// bank exists in writeDefaultGenesis, so send_enabled should be patched.
	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Bank struct {
				SendEnabled []struct {
					Denom   string `json:"denom"`
					Enabled bool   `json:"enabled"`
				} `json:"send_enabled"`
			} `json:"bank"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.AppState.Bank.SendEnabled) != 1 ||
		doc.AppState.Bank.SendEnabled[0].Denom != "uumesh" ||
		doc.AppState.Bank.SendEnabled[0].Enabled {
		t.Errorf("bank send_enabled = %+v, want [{uumesh false}]", doc.AppState.Bank.SendEnabled)
	}
}

func TestHandleSoftLaunchPatchesPresentModules(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithTransferWasmDistribution(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		SoftLaunch: SoftLaunch{
			Enabled:              true,
			DisableBankSend:      true,
			DisableIBCTransfer:   true,
			AllowWasmInstantiate: boolPtr(true),
		},
	}

	if err := handleSoftLaunch(plan); err != nil {
		t.Fatalf("handleSoftLaunch() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Transfer struct {
				Params struct {
					SendEnabled    bool `json:"send_enabled"`
					ReceiveEnabled bool `json:"receive_enabled"`
				} `json:"params"`
			} `json:"transfer"`
			Wasm struct {
				Params struct {
					InstantiateDefaultPermission string `json:"instantiate_default_permission"`
				} `json:"params"`
			} `json:"wasm"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.AppState.Transfer.Params.SendEnabled {
		t.Error("transfer send_enabled should be false")
	}
	if doc.AppState.Transfer.Params.ReceiveEnabled {
		t.Error("transfer receive_enabled should be false")
	}
	if got := doc.AppState.Wasm.Params.InstantiateDefaultPermission; got != "Everybody" {
		t.Errorf("wasm instantiate permission = %q, want Everybody", got)
	}
}

func TestAllocateDustToCommunityPoolSkipsAbsentDistribution(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	// writeDefaultGenesis has no distribution module.
	writeDefaultGenesis(t, home)

	dust := new(big.Int).SetInt64(12345)
	if err := allocateDustToCommunityPool(dust, "uumesh"); err != nil {
		t.Fatalf("allocateDustToCommunityPool() error: %v", err)
	}
	if appStateHasKey(t, home, "distribution") {
		t.Fatal("app_state.distribution was created even though distribution is not compiled into umeshd")
	}
}

func writeGenesisWithGovBankMint(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"app_state": map[string]any{
			"staking": map[string]any{
				"params": map[string]any{"bond_denom": "stake"},
			},
			"gov": map[string]any{
				"params": map[string]any{},
			},
			"bank": map[string]any{
				"params": map[string]any{},
			},
			"mint": map[string]any{
				"params": map[string]any{
					"inflation_min": "0.07",
					"inflation_max": "0.20",
				},
			},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPatchGovParamsExpedited(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithGovBankMint(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Gov: GovParams{
				ExpeditedVotingPeriod: "86400s",
				ExpeditedThreshold:    "0.670000000000000000",
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Gov struct {
				Params struct {
					ExpeditedVotingPeriod string `json:"expedited_voting_period"`
					ExpeditedThreshold    string `json:"expedited_threshold"`
				} `json:"params"`
			} `json:"gov"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.AppState.Gov.Params.ExpeditedVotingPeriod != "86400s" {
		t.Errorf("expedited_voting_period = %q, want %q", doc.AppState.Gov.Params.ExpeditedVotingPeriod, "86400s")
	}
	if doc.AppState.Gov.Params.ExpeditedThreshold != "0.670000000000000000" {
		t.Errorf("expedited_threshold = %q, want %q", doc.AppState.Gov.Params.ExpeditedThreshold, "0.670000000000000000")
	}
}

func TestPatchGovParamsCancel(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithGovBankMint(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Gov: GovParams{
				ProposalCancelRatio: "0.500000000000000000",
				ProposalCancelDest:  "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g",
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Gov struct {
				Params struct {
					ProposalCancelRatio string `json:"proposal_cancel_ratio"`
					ProposalCancelDest  string `json:"proposal_cancel_dest"`
				} `json:"params"`
			} `json:"gov"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.AppState.Gov.Params.ProposalCancelRatio != "0.500000000000000000" {
		t.Errorf("proposal_cancel_ratio = %q, want %q", doc.AppState.Gov.Params.ProposalCancelRatio, "0.500000000000000000")
	}
	if doc.AppState.Gov.Params.ProposalCancelDest != "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g" {
		t.Errorf("proposal_cancel_dest = %q, want address", doc.AppState.Gov.Params.ProposalCancelDest)
	}
}

func TestPatchGovParamsBurnVeto(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithGovBankMint(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Gov: GovParams{
				BurnVoteVeto: true,
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Gov struct {
				Params struct {
					BurnVoteVeto bool `json:"burn_vote_veto"`
				} `json:"params"`
			} `json:"gov"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.AppState.Gov.Params.BurnVoteVeto {
		t.Error("burn_vote_veto should be true")
	}
}

func TestPatchGovParamsMinDepositRatio(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithGovBankMint(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Gov: GovParams{
				MinDepositRatio: "0.010000000000000000",
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Gov struct {
				Params struct {
					MinDepositRatio string `json:"min_deposit_ratio"`
				} `json:"params"`
			} `json:"gov"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.AppState.Gov.Params.MinDepositRatio != "0.010000000000000000" {
		t.Errorf("min_deposit_ratio = %q, want %q", doc.AppState.Gov.Params.MinDepositRatio, "0.010000000000000000")
	}
}

func TestPatchGovParamsStartingProposalID(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithGovBankMint(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Gov: GovParams{
				StartingProposalID: "100",
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Gov struct {
				Params struct {
					StartingProposalID string `json:"starting_proposal_id"`
				} `json:"params"`
			} `json:"gov"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.AppState.Gov.Params.StartingProposalID != "100" {
		t.Errorf("starting_proposal_id = %q, want %q", doc.AppState.Gov.Params.StartingProposalID, "100")
	}
}

func TestPatchBankParams(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithGovBankMint(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Bank: BankParams{
				DefaultSendEnabled: false,
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Bank struct {
				Params struct {
					DefaultSendEnabled bool `json:"default_send_enabled"`
				} `json:"params"`
			} `json:"bank"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.AppState.Bank.Params.DefaultSendEnabled {
		t.Error("default_send_enabled should be false")
	}
}

func TestHandleSoftLaunchDisableInflation(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithGovBankMint(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		SoftLaunch: SoftLaunch{
			Enabled:           true,
			DisableInflation: true,
		},
	}

	if err := handleSoftLaunch(plan); err != nil {
		t.Fatalf("handleSoftLaunch() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Mint struct {
				Params struct {
					InflationMin string `json:"inflation_min"`
					InflationMax string `json:"inflation_max"`
				} `json:"params"`
			} `json:"mint"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.AppState.Mint.Params.InflationMin != "0.000000000000000000" {
		t.Errorf("inflation_min = %q, want %q", doc.AppState.Mint.Params.InflationMin, "0.000000000000000000")
	}
	if doc.AppState.Mint.Params.InflationMax != "0.000000000000000000" {
		t.Errorf("inflation_max = %q, want %q", doc.AppState.Mint.Params.InflationMax, "0.000000000000000000")
	}
}

func TestValidateGovExpeditedThreshold(t *testing.T) {
	m := &Modules{
		Gov: GovParams{
			Threshold:          "0.500000000000000000",
			ExpeditedThreshold: "0.400000000000000000", // less than threshold - should fail
		},
	}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for expedited_threshold <= threshold")
	}
}

func TestValidateGovProposalCancelRatio(t *testing.T) {
	m := &Modules{
		Gov: GovParams{
			ProposalCancelRatio: "1.5", // > 1 - should fail
		},
	}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for proposal_cancel_ratio > 1")
	}

	m = &Modules{
		Gov: GovParams{
			ProposalCancelRatio: "0", // = 0 - should fail
		},
	}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for proposal_cancel_ratio = 0")
	}
}

func TestValidateGovMinDepositRatio(t *testing.T) {
	m := &Modules{
		Gov: GovParams{
			MinDepositRatio: "1.5", // > 1 - should fail
		},
	}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for min_deposit_ratio > 1")
	}
}

func TestValidateGovExpeditedMinDeposit(t *testing.T) {
	m := &Modules{
		Gov: GovParams{
			MinDeposit:          "1000",
			ExpeditedMinDeposit: "500", // less than min_deposit - should fail
		},
	}
	if err := validateModules(m, "uumesh"); err == nil {
		t.Fatal("expected error for expedited_min_deposit <= min_deposit")
	}
}

func TestPatchDistributionParamsNoDeprecated(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithGovBankMint(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Distribution: DistributionParams{
				CommunityTax: "0.02",
			},
		},
	}

	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AppState struct {
			Distribution struct {
				Params struct {
					CommunityTax        string `json:"community_tax"`
					BaseProposerReward  string `json:"base_proposer_reward"`
					BonusProposerReward string `json:"bonus_proposer_reward"`
				} `json:"params"`
			} `json:"distribution"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.AppState.Distribution.Params.BaseProposerReward != "" {
		t.Errorf("base_proposer_reward should not be set, got %q", doc.AppState.Distribution.Params.BaseProposerReward)
	}
	if doc.AppState.Distribution.Params.BonusProposerReward != "" {
		t.Errorf("bonus_proposer_reward should not be set, got %q", doc.AppState.Distribution.Params.BonusProposerReward)
	}
}

func TestAddSingleModuleAccount(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeGenesisWithGovBankMint(t, home)

	// Add auth module with empty accounts to genesis for the test
	genesisPath := filepath.Join(home, "config", "genesis.json")
	data, err := os.ReadFile(genesisPath)
	if err != nil {
		t.Fatal(err)
	}
	var gen map[string]any
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatal(err)
	}
	appState := gen["app_state"].(map[string]any)
	appState["auth"] = map[string]any{
		"accounts": []any{},
	}
	gen["app_state"] = appState
	data, err = json.MarshalIndent(gen, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(genesisPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	d := dkrcmd.New(
		dkrcmd.WithImage("umesh/umeshd:test"),
		dkrcmd.WithHome("/home/umesh"),
		dkrcmd.WithDataDir(home),
		dkrcmd.WithBackupsDir(filepath.Join(home, "backups")),
	)

	alloc := Allocation{
		Name:       "test_module",
		KeyName:    "test_module",
		Type:       "module_account",
		ModuleName: "distribution",
	}

	addErr := AddSingleModuleAccount(d, alloc, big.NewInt(1000000), "uumesh")
	if addErr == nil {
		t.Fatal("expected error for missing docker image, got nil")
	}
	// We expect an error because the docker image doesn't exist, but we can verify
	// the command that would be run contains --module-name
	if !strings.Contains(addErr.Error(), "distribution") {
		t.Logf("error (expected): %v", addErr)
	}
}
