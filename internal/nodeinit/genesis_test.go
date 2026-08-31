package nodeinit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper: write a genesis.json with bond_denom=stake (default Cosmos SDK output).
func writeDefaultGenesis(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	doc := GenesisDocument{}
	doc.AppState.Staking.Params.BondDenom = "stake"
	doc.AppState.Mint.Params.MintDenom = "stake"
	doc.AppState.Gov.Params.MinDeposit = []GenesisDeposit{
		{Denom: "stake", Amount: "10000000"},
	}
	doc.AppState.Gov.Params.ExpeditedMinDeposit = []GenesisDeposit{
		{Denom: "stake", Amount: "50000000"},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSetGenesisTimePreservedAcrossPatches(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	// Simulate ExecutePlan Step 1b: apply the plan's genesis_time.
	if err := SetGenesisTime("2026-08-15T00:00:00Z"); err != nil {
		t.Fatalf("SetGenesisTime() error: %v", err)
	}

	// Simulate later ExecutePlan steps that rewrite the whole genesis doc.
	if err := patchDenom("uumesh"); err != nil {
		t.Fatalf("patchDenom() error: %v", err)
	}
	if err := addBankMetadata("uumesh", ""); err != nil {
		t.Fatalf("addBankMetadata() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if gt, ok := result["genesis_time"].(string); !ok || gt != "2026-08-15T00:00:00Z" {
		t.Errorf("genesis_time = %v, want 2026-08-15T00:00:00Z", result["genesis_time"])
	}
}

func TestPatchDenom(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	if err := patchDenom("uumesh"); err != nil {
		t.Fatalf("patchDenom() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatal(err)
	}

	if gen.AppState.Staking.Params.BondDenom != "uumesh" {
		t.Errorf("bond_denom = %q, want uumesh", gen.AppState.Staking.Params.BondDenom)
	}
	if gen.AppState.Mint.Params.MintDenom != "uumesh" {
		t.Errorf("mint_denom = %q, want uumesh", gen.AppState.Mint.Params.MintDenom)
	}
	for i, d := range gen.AppState.Gov.Params.MinDeposit {
		if d.Denom != "uumesh" {
			t.Errorf("min_deposit[%d].denom = %q, want uumesh", i, d.Denom)
		}
	}
	for i, d := range gen.AppState.Gov.Params.ExpeditedMinDeposit {
		if d.Denom != "uumesh" {
			t.Errorf("expedited_min_deposit[%d].denom = %q, want uumesh", i, d.Denom)
		}
	}
}

func TestPatchDenomIdempotent(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	// First call patches.
	if err := patchDenom("uumesh"); err != nil {
		t.Fatalf("first patchDenom() error: %v", err)
	}
	// Second call is no-op.
	if err := patchDenom("uumesh"); err != nil {
		t.Fatalf("second patchDenom() error: %v", err)
	}

	// Verify no double-write issues by re-reading.
	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatal(err)
	}
	if gen.AppState.Staking.Params.BondDenom != "uumesh" {
		t.Errorf("bond_denom = %q, want uumesh", gen.AppState.Staking.Params.BondDenom)
	}
}

func TestPatchDenomPreservesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	// Genesis with extra unknown fields.
	raw := `{
		"chain_id": "test-1",
		"app_state": {
			"staking": {"params": {"bond_denom": "stake"}},
			"mint": {"params": {"mint_denom": "stake"}},
			"gov": {"params": {"min_deposit": [{"denom": "stake", "amount": "100"}]}},
			"bank": {"balances": [{"address": "addr1", "coins": [{"denom": "stake", "amount": "1000"}]}]}
		}
	}`
	path := filepath.Join(home, "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := patchDenom("uumesh"); err != nil {
		t.Fatalf("patchDenom() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	// chain_id preserved.
	if cid, ok := result["chain_id"].(string); !ok || cid != "test-1" {
		t.Errorf("chain_id = %v, want test-1", result["chain_id"])
	}
	// bank.balances preserved.
	appState, ok := result["app_state"].(map[string]interface{})
	if !ok {
		t.Fatal("app_state missing or not an object")
	}
	bank, ok := appState["bank"].(map[string]interface{})
	if !ok {
		t.Fatal("bank missing or not an object")
	}
	if _, ok := bank["balances"].([]interface{}); !ok {
		t.Error("bank.balances not preserved")
	}
}

func TestPatchDenomNoStakeRemains(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	// Genesis with "stake" spread across module params AND bank balances.
	raw := `{
		"chain_id": "test-1",
		"app_state": {
			"staking": {"params": {"bond_denom": "stake"}},
			"mint": {"params": {"mint_denom": "stake"}},
			"gov": {
				"params": {
					"min_deposit": [{"denom": "stake", "amount": "100"}],
					"expedited_min_deposit": [{"denom": "stake", "amount": "500"}]
				}
			},
			"bank": {
				"balances": [{"address": "addr1", "coins": [{"denom": "stake", "amount": "1000"}]}]
			}
		}
	}`
	path := filepath.Join(home, "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := patchDenom("uumesh"); err != nil {
		t.Fatalf("patchDenom() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"stake"`) {
		t.Errorf("patchDenom left the default denom \"stake\" in the genesis:\n%s", data)
	}

	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatal(err)
	}
	if gen.AppState.Staking.Params.BondDenom != "uumesh" {
		t.Errorf("bond_denom = %q, want uumesh", gen.AppState.Staking.Params.BondDenom)
	}
	if gen.AppState.Mint.Params.MintDenom != "uumesh" {
		t.Errorf("mint_denom = %q, want uumesh", gen.AppState.Mint.Params.MintDenom)
	}
}

func TestAddBankMetadata(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	if err := addBankMetadata("uumesh", ""); err != nil {
		t.Fatalf("addBankMetadata() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatal(err)
	}

	if len(gen.AppState.Bank.DenomMetadata) != 1 {
		t.Fatalf("denom_metadata length = %d, want 1", len(gen.AppState.Bank.DenomMetadata))
	}

	meta := gen.AppState.Bank.DenomMetadata[0]
	if meta.Base != "uumesh" {
		t.Errorf("base = %q, want uumesh", meta.Base)
	}
	if meta.Display != "UMESH" {
		t.Errorf("display = %q, want UMESH (human-readable, not base denom)", meta.Display)
	}
	if meta.Name != "UMESH" {
		t.Errorf("name = %q, want UMESH", meta.Name)
	}
	if meta.Symbol != "UMESH" {
		t.Errorf("symbol = %q, want UMESH", meta.Symbol)
	}
	if len(meta.DenomUnits) != 3 {
		t.Fatalf("denom_units length = %d, want 3", len(meta.DenomUnits))
	}
	// Check micro tier.
	if meta.DenomUnits[0].Denom != "uumesh" || meta.DenomUnits[0].Exponent != 0 {
		t.Errorf("denom_units[0] = %+v, want {uumesh 0}", meta.DenomUnits[0])
	}
	// Check milli tier.
	if meta.DenomUnits[1].Denom != "mumesh" || meta.DenomUnits[1].Exponent != 3 {
		t.Errorf("denom_units[1] = %+v, want {mumesh 3}", meta.DenomUnits[1])
	}
	// Check unit tier.
	if meta.DenomUnits[2].Denom != "UMESH" || meta.DenomUnits[2].Exponent != 6 {
		t.Errorf("denom_units[2] = %+v, want {UMESH 6}", meta.DenomUnits[2])
	}
}

func TestAddBankMetadataIdempotent(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	// First call adds metadata.
	if err := addBankMetadata("uumesh", ""); err != nil {
		t.Fatalf("first addBankMetadata() error: %v", err)
	}
	// Second call is no-op.
	if err := addBankMetadata("uumesh", ""); err != nil {
		t.Fatalf("second addBankMetadata() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatal(err)
	}
	if len(gen.AppState.Bank.DenomMetadata) != 1 {
		t.Errorf("denom_metadata length = %d, want 1 (no duplicate)", len(gen.AppState.Bank.DenomMetadata))
	}
}

func TestAddBankMetadataSkipsIfPresent(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	// Genesis with existing metadata.
	raw := `{
		"app_state": {
			"staking": {"params": {"bond_denom": "uumesh"}},
			"mint": {"params": {"mint_denom": "uumesh"}},
			"gov": {"params": {}},
			"bank": {"denom_metadata": [{"base": "existing", "display": "existing"}]}
		}
	}`
	path := filepath.Join(home, "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := addBankMetadata("uumesh", ""); err != nil {
		t.Fatalf("addBankMetadata() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatal(err)
	}
	if len(gen.AppState.Bank.DenomMetadata) != 1 {
		t.Fatalf("denom_metadata length = %d, want 1 (should not add)", len(gen.AppState.Bank.DenomMetadata))
	}
	if gen.AppState.Bank.DenomMetadata[0].Base != "existing" {
		t.Errorf("base = %q, want existing (original preserved)", gen.AppState.Bank.DenomMetadata[0].Base)
	}
}

func TestAddBankMetadataFromPlanDisplay(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	plan := &Plan{Chain: ChainConfig{Denom: "uumesh", DenomURI: "https://github.com/opscores/node-umesh"}}
	if err := addBankMetadataFromPlan(plan); err != nil {
		t.Fatalf("addBankMetadataFromPlan() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		t.Fatal(err)
	}
	if len(gen.AppState.Bank.DenomMetadata) != 1 {
		t.Fatalf("denom_metadata length = %d, want 1", len(gen.AppState.Bank.DenomMetadata))
	}
	meta := gen.AppState.Bank.DenomMetadata[0]
	if meta.Base != "uumesh" {
		t.Errorf("base = %q, want uumesh", meta.Base)
	}
	if meta.Display != "UMESH" {
		t.Errorf("display = %q, want UMESH (human-readable, not base denom)", meta.Display)
	}
	if meta.Name != "UMESH" {
		t.Errorf("name = %q, want UMESH", meta.Name)
	}
	if meta.Symbol != "UMESH" {
		t.Errorf("symbol = %q, want UMESH", meta.Symbol)
	}
	if meta.URI != "https://github.com/opscores/node-umesh" {
		t.Errorf("uri = %q, want plan denom_uri", meta.URI)
	}
}

func TestPatchModuleParamsNewFields(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	plan := &Plan{
		Chain: ChainConfig{Denom: "uumesh"},
		Modules: Modules{
			Staking: StakingParams{MinCommissionRate: "0.050000000000000000"},
			Mint:    MintParams{MaxSupply: "1000000000000000"},
			Gov:     GovParams{MinInitialDepositRatio: "0.100000000000000000"},
		},
	}
	if err := patchModuleParams(plan); err != nil {
		t.Fatalf("patchModuleParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	app := doc["app_state"].(map[string]any)
	if got := app["staking"].(map[string]any)["params"].(map[string]any)["min_commission_rate"]; got != "0.050000000000000000" {
		t.Errorf("min_commission_rate = %v, want 0.05", got)
	}
	if got := app["mint"].(map[string]any)["params"].(map[string]any)["max_supply"]; got != "1000000000000000" {
		t.Errorf("max_supply = %v, want 1000000000000000", got)
	}
	if got := app["gov"].(map[string]any)["params"].(map[string]any)["min_initial_deposit_ratio"]; got != "0.100000000000000000" {
		t.Errorf("min_initial_deposit_ratio = %v, want 0.1", got)
	}
}

func TestPatchConsensusParamsAuthority(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	// writeDefaultGenesis has no top-level "consensus" key; inject the modern
	// layout so patchConsensusParams takes the modern path.
	if err := PatchGenesisParam("consensus.params.authority.authority", `""`); err != nil {
		t.Fatalf("seed authority: %v", err)
	}

	err := patchConsensusParams(ConsensusParams{
		Authority: "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g",
	})
	if err != nil {
		t.Fatalf("patchConsensusParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	consensus, ok := result["consensus"].(map[string]any)
	if !ok {
		t.Fatal("consensus key missing")
	}
	params := consensus["params"].(map[string]any)
	auth := params["authority"].(map[string]any)
	if auth["authority"] != "umesh10d07y265gmmuvt4z0w9aw880jnsr700jplz74g" {
		t.Errorf("authority = %v, want gov address", auth["authority"])
	}
	if params["block"].(map[string]any)["max_gas"] != "-1" {
		t.Errorf("max_gas = %v, want default -1", params["block"].(map[string]any)["max_gas"])
	}
}

func TestPatchConsensusParamsAuthorityUnset(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)
	writeDefaultGenesis(t, home)

	if err := PatchGenesisParam("consensus.params.authority.authority", `"keep-me"`); err != nil {
		t.Fatalf("seed authority: %v", err)
	}

	// Empty Authority must leave the existing value untouched.
	if err := patchConsensusParams(ConsensusParams{}); err != nil {
		t.Fatalf("patchConsensusParams() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	auth := result["consensus"].(map[string]any)["params"].(map[string]any)["authority"].(map[string]any)
	if auth["authority"] != "keep-me" {
		t.Errorf("authority = %v, want existing value preserved", auth["authority"])
	}
}

func TestValidateGenesisAmounts(t *testing.T) {
	tests := []struct {
		name           string
		stake          string
		self           string
		wantErr        bool
	}{
		{"equal amounts", "1000uumesh", "1000uumesh", false},
		{"self > stake", "1000uumesh", "2000uumesh", false},
		{"self < stake", "2000uumesh", "1000uumesh", true},
		{"large amounts equal", "1000000000000uumesh", "1000000000000uumesh", false},
		{"large amounts self less", "1000000000000uumesh", "999999999999uumesh", true},
		{"empty stake ignored", "", "1000uumesh", false},
		{"empty self ignored", "1000uumesh", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGenesisAmounts(tc.stake, tc.self)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateGenesisAmounts(%q, %q) error = %v, wantErr = %v", tc.stake, tc.self, err, tc.wantErr)
			}
		})
	}
}
