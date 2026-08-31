package nodeinit

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeinfo"
	"github.com/opscores/umesh-cli/internal/tune"
)

// GenesisDocument wraps the genesis.json top-level object. All unknown
// top-level fields are preserved verbatim via RawMessage; only app_state is
// decoded into a typed struct for modification.
type GenesisDocument struct {
	raw      map[string]json.RawMessage
	AppState GenesisAppState `json:"-"`
}

// GenesisAppState holds the app_state fields we need to modify while
// preserving unknown fields (e.g. bank.balances, auth.accounts).
type GenesisAppState struct {
	raw     map[string]json.RawMessage
	Staking GenesisStaking `json:"-"`
	Mint    GenesisMint    `json:"-"`
	Gov     GenesisGov     `json:"-"`
	Bank    GenesisBank    `json:"-"`
}

// UnmarshalJSON decodes app_state preserving all unknown sub-fields.
func (g *GenesisAppState) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &g.raw); err != nil {
		return err
	}
	if raw, ok := g.raw["staking"]; ok {
		if err := json.Unmarshal(raw, &g.Staking); err != nil {
			return err
		}
	}
	if raw, ok := g.raw["mint"]; ok {
		if err := json.Unmarshal(raw, &g.Mint); err != nil {
			return err
		}
	}
	if raw, ok := g.raw["gov"]; ok {
		if err := json.Unmarshal(raw, &g.Gov); err != nil {
			return err
		}
	}
	if raw, ok := g.raw["bank"]; ok {
		if err := json.Unmarshal(raw, &g.Bank); err != nil {
			return err
		}
	}
	return nil
}

// MarshalJSON reconstructs app_state preserving unknown sub-fields.
func (g GenesisAppState) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(g.raw))
	for k, v := range g.raw {
		out[k] = v
	}
	for _, kv := range []struct {
		key string
		val interface{}
	}{
		{"staking", g.Staking},
		{"mint", g.Mint},
		{"gov", g.Gov},
		{"bank", g.Bank},
	} {
		data, err := json.Marshal(kv.val)
		if err != nil {
			return nil, err
		}
		out[kv.key] = data
	}
	return json.Marshal(out)
}

// GenesisStaking holds staking.params while preserving unknown params fields.
type GenesisStaking struct {
	raw    map[string]json.RawMessage
	Params GenesisParamsStaking `json:"-"`
}

// GenesisParamsStaking holds params.bond_denom.
type GenesisParamsStaking struct {
	raw       map[string]json.RawMessage
	BondDenom string `json:"-"`
}

// UnmarshalJSON decodes staking preserving unknown fields.
func (g *GenesisStaking) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &g.raw); err != nil {
		return err
	}
	if raw, ok := g.raw["params"]; ok {
		return json.Unmarshal(raw, &g.Params)
	}
	return nil
}

// MarshalJSON reconstructs staking preserving unknown fields.
func (g GenesisStaking) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(g.raw))
	for k, v := range g.raw {
		out[k] = v
	}
	data, err := json.Marshal(g.Params)
	if err != nil {
		return nil, err
	}
	out["params"] = data
	return json.Marshal(out)
}

// UnmarshalJSON decodes params preserving unknown fields.
func (g *GenesisParamsStaking) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &g.raw); err != nil {
		return err
	}
	if raw, ok := g.raw["bond_denom"]; ok {
		return json.Unmarshal(raw, &g.BondDenom)
	}
	return nil
}

// MarshalJSON reconstructs params preserving unknown fields.
func (g GenesisParamsStaking) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(g.raw))
	for k, v := range g.raw {
		out[k] = v
	}
	data, err := json.Marshal(g.BondDenom)
	if err != nil {
		return nil, err
	}
	out["bond_denom"] = data
	return json.Marshal(out)
}

// GenesisMint holds mint.params while preserving unknown params fields.
type GenesisMint struct {
	raw    map[string]json.RawMessage
	Params GenesisParamsMint `json:"-"`
}

// GenesisParamsMint holds params.mint_denom.
type GenesisParamsMint struct {
	raw       map[string]json.RawMessage
	MintDenom string `json:"-"`
}

// UnmarshalJSON decodes mint preserving unknown fields.
func (g *GenesisMint) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &g.raw); err != nil {
		return err
	}
	if raw, ok := g.raw["params"]; ok {
		return json.Unmarshal(raw, &g.Params)
	}
	return nil
}

// MarshalJSON reconstructs mint preserving unknown fields.
func (g GenesisMint) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(g.raw))
	for k, v := range g.raw {
		out[k] = v
	}
	data, err := json.Marshal(g.Params)
	if err != nil {
		return nil, err
	}
	out["params"] = data
	return json.Marshal(out)
}

// UnmarshalJSON decodes params preserving unknown fields.
func (g *GenesisParamsMint) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &g.raw); err != nil {
		return err
	}
	if raw, ok := g.raw["mint_denom"]; ok {
		return json.Unmarshal(raw, &g.MintDenom)
	}
	return nil
}

// MarshalJSON reconstructs params preserving unknown fields.
func (g GenesisParamsMint) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(g.raw))
	for k, v := range g.raw {
		out[k] = v
	}
	data, err := json.Marshal(g.MintDenom)
	if err != nil {
		return nil, err
	}
	out["mint_denom"] = data
	return json.Marshal(out)
}

// GenesisGov holds gov.params while preserving unknown params fields.
type GenesisGov struct {
	raw    map[string]json.RawMessage
	Params GenesisParamsGov `json:"-"`
}

// GenesisParamsGov holds params.min_deposit and expedited_min_deposit.
type GenesisParamsGov struct {
	raw                 map[string]json.RawMessage
	MinDeposit          []GenesisDeposit `json:"-"`
	ExpeditedMinDeposit []GenesisDeposit `json:"-"`
}

// UnmarshalJSON decodes gov preserving unknown fields.
func (g *GenesisGov) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &g.raw); err != nil {
		return err
	}
	if raw, ok := g.raw["params"]; ok {
		return json.Unmarshal(raw, &g.Params)
	}
	return nil
}

// MarshalJSON reconstructs gov preserving unknown fields.
func (g GenesisGov) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(g.raw))
	for k, v := range g.raw {
		out[k] = v
	}
	data, err := json.Marshal(g.Params)
	if err != nil {
		return nil, err
	}
	out["params"] = data
	return json.Marshal(out)
}

// UnmarshalJSON decodes params preserving unknown fields.
func (g *GenesisParamsGov) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &g.raw); err != nil {
		return err
	}
	if raw, ok := g.raw["min_deposit"]; ok {
		if err := json.Unmarshal(raw, &g.MinDeposit); err != nil {
			return err
		}
	}
	if raw, ok := g.raw["expedited_min_deposit"]; ok {
		return json.Unmarshal(raw, &g.ExpeditedMinDeposit)
	}
	return nil
}

// MarshalJSON reconstructs params preserving unknown fields.
func (g GenesisParamsGov) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(g.raw))
	for k, v := range g.raw {
		out[k] = v
	}
	for _, kv := range []struct {
		key string
		val interface{}
	}{
		{"min_deposit", g.MinDeposit},
		{"expedited_min_deposit", g.ExpeditedMinDeposit},
	} {
		data, err := json.Marshal(kv.val)
		if err != nil {
			return nil, err
		}
		out[kv.key] = data
	}
	return json.Marshal(out)
}

// GenesisDeposit represents a single denom+amount deposit entry.
type GenesisDeposit struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

// GenesisBank holds bank.denom_metadata while preserving unknown fields
// (e.g. balances, supply, params).
type GenesisBank struct {
	raw           map[string]json.RawMessage
	DenomMetadata []GenesisDenomMetadata `json:"-"`
}

// UnmarshalJSON decodes bank preserving all unknown sub-fields.
func (g *GenesisBank) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &g.raw); err != nil {
		return err
	}
	if raw, ok := g.raw["denom_metadata"]; ok {
		return json.Unmarshal(raw, &g.DenomMetadata)
	}
	return nil
}

// MarshalJSON reconstructs bank preserving unknown sub-fields.
func (g GenesisBank) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(g.raw))
	for k, v := range g.raw {
		out[k] = v
	}
	data, err := json.Marshal(g.DenomMetadata)
	if err != nil {
		return nil, err
	}
	out["denom_metadata"] = data
	return json.Marshal(out)
}

// GenesisDenomMetadata describes a single denom for wallets/explorers.
type GenesisDenomMetadata struct {
	Description string             `json:"description"`
	DenomUnits  []GenesisDenomUnit `json:"denom_units"`
	Base        string             `json:"base"`
	Display     string             `json:"display"`
	Name        string             `json:"name"`
	Symbol      string             `json:"symbol"`
	URI         string             `json:"uri"`
	URIHash     string             `json:"uri_hash"`
}

// GenesisDenomUnit represents one exponent tier of a denom.
type GenesisDenomUnit struct {
	Denom    string   `json:"denom"`
	Exponent uint32   `json:"exponent"`
	Aliases  []string `json:"aliases"`
}

// UnmarshalJSON decodes the genesis document while preserving all top-level
// fields that umeshctl does not modify (e.g. chain_id, genesis_time).
func (g *GenesisDocument) UnmarshalJSON(data []byte) error {
	// Decode all top-level fields into raw map.
	if err := json.Unmarshal(data, &g.raw); err != nil {
		return err
	}
	// Decode app_state into typed struct.
	if rawApp, ok := g.raw["app_state"]; ok {
		return json.Unmarshal(rawApp, &g.AppState)
	}
	return nil
}

// MarshalJSON reconstructs the full genesis document preserving field order.
func (g GenesisDocument) MarshalJSON() ([]byte, error) {
	// Clone the raw map so we don't mutate the original.
	out := make(map[string]json.RawMessage, len(g.raw))
	for k, v := range g.raw {
		out[k] = v
	}
	// Re-encode app_state from the typed struct.
	appState, err := json.Marshal(g.AppState)
	if err != nil {
		return nil, err
	}
	out["app_state"] = appState
	return json.Marshal(out)
}

// GenesisParams carries env values for the init-genesis flow.
type GenesisParams struct {
	ChainID                    string
	Moniker                    string
	Denom                      string
	MinGasPrice                string
	Environment                string
	KeyringPass                string
	ValidatorName              string
	StakeAmount                string
	SelfDelegation             string
	ExternalIP                 string
	CommissionRate             string
	CommissionMaxRate          string
	CommissionMaxChange        string
	ValidatorMinSelfDelegation string
	DenomURI                   string
}

// RunGenesis executes the init-genesis flow: init, denom patching, gentx, and
// final genesis collection. Runs on the host via docker run --rm. Idempotent:
// returns ErrAlreadyInitialized if genesis.json already exists.
func RunGenesis(p GenesisParams) error {
	if err := AbortIfInitialized(); err != nil {
		return err
	}
	if err := ValidateCommon(p.ChainID, p.Moniker, p.Denom, p.MinGasPrice, p.Environment); err != nil {
		return err
	}
	if err := validateKeyName(p.ValidatorName); err != nil {
		return err
	}
	if p.KeyringPass == "" {
		return fmt.Errorf("keyring password is required (use --keyring-password-file, --keyring-password-stdin, or --keyring-password-exec)")
	}
	if p.StakeAmount != "" && p.SelfDelegation != "" {
		if err := validateGenesisAmounts(p.StakeAmount, p.SelfDelegation); err != nil {
			return err
		}
	}
	if p.ExternalIP != "" {
		if err := ValidatePrivateIP(p.ExternalIP, false); err != nil {
			return err
		}
	}

	d := docker()
	if err := d.Preflight(); err != nil {
		return err
	}

	// genesis mode: block 1 chain restart. Stale validator state from a prior
	// run makes CometBFT v0.39 panic with "error replaying blocks".
	// Zero state on --force, delete priv_validator_key.json so the validator
	// key is regenerated and the node does not double-sign.
	if err := umeshdInit(d, p.Moniker, p.ChainID, Home(), false); err != nil {
		return err
	}
	if err := tune.Apply(ConfigDir(), tune.RoleGenesis, tune.Options{Moniker: p.Moniker, Environment: p.Environment, Denom: p.Denom, MinGasPrice: p.MinGasPrice, ExternalAddress: p.ExternalIP, ChainID: p.ChainID}); err != nil {
		return err
	}

	if err := patchDenom(p.Denom); err != nil {
		return err
	}
	if err := addBankMetadata(p.Denom, p.DenomURI); err != nil {
		return err
	}

	valAddr, err := createValidatorAccount(d, p.ValidatorName, p.KeyringPass)
	if err != nil {
		return err
	}
	if err := addGenesisValidatorAccount(d, valAddr, p.SelfDelegation, p.Denom); err != nil {
		return err
	}
	if err := generateGentx(d, p.ValidatorName, p.KeyringPass, p.StakeAmount, p.Denom, p.ExternalIP, p.ChainID, p.Moniker, p.CommissionRate, p.CommissionMaxRate, p.CommissionMaxChange, p.ValidatorMinSelfDelegation); err != nil {
		return err
	}
	// Advertise the reachable P2P address in config.toml too. Inside a
	// container the node would otherwise introspect its bridge IP, which peers
	// on other VPS hosts cannot dial.
	if p.ExternalIP != "" {
		if err := setExternalAddress(p.ExternalIP); err != nil {
			return err
		}
	}
	if err := collectGentxs(d); err != nil {
		return err
	}
	if err := validateGenesis(d); err != nil {
		return err
	}

	info := nodeinfo.Info{
		ChainID:            p.ChainID,
		Mode:               "validator",
		ValidatorOperator:  valoperFromKeyring(d, p.ValidatorName, p.KeyringPass),
		ValidatorAddress:   localValidatorAddr(Home()),
		NodeID:             localNodeID(Home()),
		KeyringBackend:     "file",
		GenesisTime:        extractGenesisTime(GenesisFile()),
		AutoAccountCreated: true,
		ValidatorReady:     1,
	}
	return nodeinfo.Write(Home(), info)
}

// containerHome returns the in-container home path for docker commands.
func containerHome(d *dkrcmd.Docker) string {
	return d.Home
}

// extractAmount returns the numeric prefix of a coin amount (e.g. "1000uumesh" → "1000").
func ExtractAmount(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// validateGenesisAmounts ensures self-delegation is not less than the staked
// amount. gentx fails deep in the flow with "insufficient account funds" when
// the account balance is below the staked amount, so catch it early. Only the
// numeric prefixes are compared; denom mismatches are irrelevant to ordering.
func validateGenesisAmounts(stakeAmount, selfDelegation string) error {
	stake, stakeOK := new(big.Int).SetString(ExtractAmount(stakeAmount), 10)
	self, selfOK := new(big.Int).SetString(ExtractAmount(selfDelegation), 10)
	if stakeOK && selfOK && self.Cmp(stake) < 0 {
		return fmt.Errorf("selfDelegation (%s) must be >= stakeAmount (%s)", selfDelegation, stakeAmount)
	}
	return nil
}

// patchDenom patches all denom fields in genesis.json to match DENOM.
// Idempotent: returns nil immediately if bond_denom already equals denom.
func patchDenom(denom string) error {
	path := filepath.Join(Home(), "config", "genesis.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read genesis: %w", err)
	}
	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		return fmt.Errorf("parse genesis: %w", err)
	}

	// Idempotent: skip if already patched.
	if gen.AppState.Staking.Params.BondDenom == denom {
		return nil
	}

	gen.AppState.Staking.Params.BondDenom = denom
	gen.AppState.Mint.Params.MintDenom = denom
	for i := range gen.AppState.Gov.Params.MinDeposit {
		gen.AppState.Gov.Params.MinDeposit[i].Denom = denom
	}
	for i := range gen.AppState.Gov.Params.ExpeditedMinDeposit {
		gen.AppState.Gov.Params.ExpeditedMinDeposit[i].Denom = denom
	}

	if err := writeGenesis(path, &gen); err != nil {
		return err
	}

	// Safety net: a freshly initialized genesis references the SDK default
	// denom "stake" only in the module params patched above, but sweep the
	// whole document so a wasmd module change can never ship a stale
	// default-denom reference (e.g. bank balances, a future module).
	return sweepDefaultDenom(path, denom)
}

// sweepDefaultDenom rewrites any string value exactly equal to "stake" (the
// SDK default bond denom) to the target denom across the whole genesis
// document.
func sweepDefaultDenom(path, denom string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read genesis for denom sweep: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse genesis for denom sweep: %w", err)
	}
	if replaceDenomValue(doc, denom) == 0 {
		return nil
	}
	return writeJSONMap(path, doc)
}

// replaceDenomValue recursively replaces strings exactly equal to the SDK
// default denom "stake" and returns how many replacements were made.
func replaceDenomValue(v any, denom string) int {
	switch t := v.(type) {
	case string:
		if t == "stake" {
			return 1
		}
		return 0
	case map[string]any:
		n := 0
		for k, val := range t {
			if s, ok := val.(string); ok && s == "stake" {
				t[k] = denom
				n++
				continue
			}
			n += replaceDenomValue(val, denom)
		}
		return n
	case []any:
		n := 0
		for i, val := range t {
			if s, ok := val.(string); ok && s == "stake" {
				t[i] = denom
				n++
				continue
			}
			n += replaceDenomValue(val, denom)
		}
		return n
	default:
		return 0
	}
}

// writeJSONMap atomically writes a decoded JSON document as indented JSON.
func writeJSONMap(path string, doc map[string]any) error {
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal genesis for denom sweep: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// addBankMetadata adds bank denom_metadata for wallets/explorers.
// Idempotent: returns nil immediately if denom_metadata already exists.
func addBankMetadata(denom, denomURI string) error {
	path := filepath.Join(Home(), "config", "genesis.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read genesis: %w", err)
	}
	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		return fmt.Errorf("parse genesis: %w", err)
	}

	// Idempotent: skip if metadata already present.
	if len(gen.AppState.Bank.DenomMetadata) > 0 {
		return nil
	}

	display := strings.TrimPrefix(denom, "u")
	metadata := GenesisDenomMetadata{
		Description: "The native token of Umesh Network",
		DenomUnits: []GenesisDenomUnit{
			{Denom: denom, Exponent: 0, Aliases: []string{"micro-" + denom}},
			{Denom: "m" + display, Exponent: 3, Aliases: []string{"milli-" + denom}},
			{Denom: strings.ToUpper(display), Exponent: 6, Aliases: []string{}},
		},
		Base:    denom,
		Display: strings.ToUpper(display),
		Name:    strings.ToUpper(display),
		Symbol:  strings.ToUpper(display),
	}
	if denomURI != "" {
		metadata.URI = denomURI
	}
	gen.AppState.Bank.DenomMetadata = []GenesisDenomMetadata{metadata}

	return writeGenesis(path, &gen)
}

// writeGenesis marshals a GenesisDocument and atomically writes it to path.
func writeGenesis(path string, gen *GenesisDocument) error {
	data, err := json.MarshalIndent(gen, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal genesis: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write genesis temp: %w", err)
	}
	return os.Rename(tmp, path)
}

// createValidatorAccount adds (or reuses) the validator key and returns its bech32 address.
func createValidatorAccount(d *dkrcmd.Docker, name, password string) (string, error) {
	out, err := keysShow(d, name, password)
	if err != nil {
		out, err = keysAdd(d, name, password)
		if err != nil {
			return "", fmt.Errorf("create validator key: %w", err)
		}
	}
	var k struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(out), &k); err != nil || k.Address == "" {
		return "", fmt.Errorf("cannot read validator address from keyring output: %s", out)
	}
	return k.Address, nil
}

// addGenesisValidatorAccount adds the account to genesis with a self-delegation.
func addGenesisValidatorAccount(d *dkrcmd.Docker, address, selfDelegation, denom string) error {
	_, err := d.RunMount(nil, "genesis", "add-genesis-account", address,
		ExtractAmount(selfDelegation)+denom, "--home", containerHome(d))
	return err
}

// generateGentx creates a gentx transaction for the validator. moniker is the
// node moniker written into the on-chain validator description.
func generateGentx(d *dkrcmd.Docker, name, password, stakeAmount, denom, externalIP, chainID, moniker, commissionRate, commissionMaxRate, commissionMaxChange, minSelfDelegation string) error {
	amount := ExtractAmount(stakeAmount) + denom
	gentxDir := containerHome(d) + "/config/gentx"
	if ForceReinit {
		_, _ = d.RunMount(nil, "sh", "-c", "rm -f "+gentxDir+"/gentx-*.json")
		hostGentx := filepath.Join(Home(), "config", "gentx")
		if entries, err := filepath.Glob(hostGentx + "/gentx-*.json"); err == nil {
			for _, f := range entries {
				_ = os.Remove(f)
			}
		}
	}
	args := []string{"genesis", "gentx", name,
		amount,
		"--chain-id", chainID,
		"--keyring-backend", "file",
		"--keyring-dir", containerHome(d) + "/keyring",
		"--home", containerHome(d),
		"--moniker", moniker,
	}
	if externalIP != "" {
		args = append(args, "--ip", externalIP)
	}
	// Commission and minimum-self-delegation env vars are optional: only pass
	// them when set, otherwise the SDK defaults apply.
	if commissionRate != "" {
		args = append(args, "--commission-rate", commissionRate)
		args = append(args, "--commission-max-rate", commissionMaxRate)
		args = append(args, "--commission-max-change-rate", commissionMaxChange)
	}
	if minSelfDelegation != "" {
		args = append(args, "--min-self-delegation", minSelfDelegation)
	}
	_, err := d.RunMount(strings.NewReader(password+"\n"), args...)
	return err
}

// collectGentxs runs `umeshd genesis collect-gentxs`.
func collectGentxs(d *dkrcmd.Docker) error {
	_, err := d.RunMount(nil, "genesis", "collect-gentxs", "--home", containerHome(d))
	return err
}

// FetchGenesisParams carries parameters for fetching a genesis document.
type FetchGenesisParams struct {
	URL     string
	SHA256  string
	ChainID string
	Denom   string
	Output  string
	// WriteFile controls whether the genesis document is written to disk.
	// Default is true for backward compatibility.
	// When false, the genesis document is returned in RawData only.
	WriteFile bool
}

// FetchGenesisResult contains extracted network parameters from genesis.
type FetchGenesisResult struct {
	ChainID   string
	BondDenom string
	// RawData contains the raw genesis document bytes (only populated when WriteFile=false or on success).
	RawData []byte
}

// chainIDOf extracts the chain_id from a genesis document.
func chainIDOf(genesis []byte) (string, error) {
	var g struct {
		ChainID string `json:"chain_id"`
	}
	if err := json.Unmarshal(genesis, &g); err != nil {
		return "", err
	}
	return g.ChainID, nil
}

// bondDenomOf extracts the staking bond_denom from a genesis document
// with fallback chain for different Cosmos SDK module layouts.
func bondDenomOf(genesis []byte) (string, error) {
	// 1. Primary: app_state.staking.params.bond_denom
	var g1 struct {
		AppState struct {
			Staking struct {
				Params struct {
					BondDenom string `json:"bond_denom"`
				} `json:"params"`
			} `json:"staking"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(genesis, &g1); err == nil && g1.AppState.Staking.Params.BondDenom != "" {
		return g1.AppState.Staking.Params.BondDenom, nil
	}

	// 2. Fallback: app_state.gov.params.min_deposit[0].denom (Gov v1)
	var g2 struct {
		AppState struct {
			Gov struct {
				Params struct {
					MinDeposit []struct {
						Denom string `json:"denom"`
					} `json:"min_deposit"`
				} `json:"params"`
			} `json:"gov"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(genesis, &g2); err == nil && len(g2.AppState.Gov.Params.MinDeposit) > 0 && g2.AppState.Gov.Params.MinDeposit[0].Denom != "" {
		return g2.AppState.Gov.Params.MinDeposit[0].Denom, nil
	}

	// 3. Fallback: app_state.gov.deposit_params.min_deposit[0].denom (Gov v1beta1)
	var g3 struct {
		AppState struct {
			Gov struct {
				DepositParams struct {
					MinDeposit []struct {
						Denom string `json:"denom"`
					} `json:"min_deposit"`
				} `json:"deposit_params"`
			} `json:"gov"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(genesis, &g3); err == nil && len(g3.AppState.Gov.DepositParams.MinDeposit) > 0 && g3.AppState.Gov.DepositParams.MinDeposit[0].Denom != "" {
		return g3.AppState.Gov.DepositParams.MinDeposit[0].Denom, nil
	}

	return "", fmt.Errorf("could not auto-detect bond_denom from genesis: neither staking nor gov module parameters contain denom info")
}

// FetchGenesis downloads a genesis document from the given URL, validates
// it (SHA256, chain-ID, bond_denom), and optionally writes it to the output path.
// Returns extracted network parameters for auto-configuration.
// When WriteFile is false, the genesis document is not written to disk but returned in RawData.
func FetchGenesis(p FetchGenesisParams) (*FetchGenesisResult, error) {
	body, err := DownloadGenesis(p.URL, 5, 5, true)
	if err != nil {
		return nil, fmt.Errorf("download genesis from %s: %w", p.URL, err)
	}

	clean, _ := UnwrapGenesisRPC(body)

	if p.SHA256 != "" {
		if err := VerifyHash(p.SHA256, SHA256Hex(clean)); err != nil {
			return nil, err
		}
	}

	if p.ChainID != "" {
		if id, err := chainIDOf(clean); err == nil && id != "" && id != p.ChainID {
			return nil, fmt.Errorf("chain-id mismatch: genesis has %q, expected %q", id, p.ChainID)
		}
	}

	// Extract bond_denom from genesis
	bondDenom, err := bondDenomOf(clean)
	if err != nil {
		// If denom was provided in params, use it as fallback
		if p.Denom != "" {
			bondDenom = p.Denom
		} else {
			return nil, fmt.Errorf("failed to extract bond_denom and no denom provided: %w", err)
		}
	}

	// Validate denom mismatch if provided in params
	if p.Denom != "" && bondDenom != p.Denom {
		return nil, fmt.Errorf("denom mismatch: YAML config specifies %q, but genesis defines %q", p.Denom, bondDenom)
	}

	// Validate chain-id from genesis if not provided
	extractedChainID := p.ChainID
	if extractedChainID == "" {
		if id, err := chainIDOf(clean); err == nil && id != "" {
			extractedChainID = id
		}
	}

	// Write to disk only if WriteFile is true (default for backward compatibility)
	if p.WriteFile {
		output := GenesisFile()
		if p.Output != "" {
			output = p.Output
		}
		if err := os.WriteFile(output, clean, 0o644); err != nil {
			return nil, err
		}
	}

	return &FetchGenesisResult{
		ChainID:   extractedChainID,
		BondDenom: bondDenom,
		RawData:   clean,
	}, nil
}

// enabledAppStateModules returns the set of top-level app_state keys from the
// generated genesis.json. After `umeshd init` these are exactly the modules
// compiled into the binary. An empty set is returned when genesis.json does
// not exist yet (e.g. in unit tests that patch a plan before init).
func enabledAppStateModules() (map[string]bool, error) {
	enabled := map[string]bool{}
	data, err := os.ReadFile(filepath.Join(Home(), "config", "genesis.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return enabled, nil
		}
		return nil, fmt.Errorf("read genesis: %w", err)
	}
	var doc struct {
		AppState map[string]json.RawMessage `json:"app_state"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse genesis: %w", err)
	}
	for k := range doc.AppState {
		enabled[k] = true
	}
	return enabled, nil
}

// PatchGenesisParam sets a nested parameter in genesis.json by dot-separated path.
// Path format: "app_state.staking.params.max_validators"
// Value is parsed as JSON (numbers, strings, booleans, objects).
func PatchGenesisParam(path, valueStr string) error {
	genesis := filepath.Join(Home(), "config", "genesis.json")
	data, err := os.ReadFile(genesis)
	if err != nil {
		return fmt.Errorf("read genesis: %w", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse genesis: %w", err)
	}

	// Parse value as JSON (supports numbers, strings, bools, objects)
	var value any
	if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
		// If not valid JSON, treat as string
		value = valueStr
	}

	// Navigate and set
	keys := strings.Split(path, ".")
	if err := setNestedValue(doc, keys, value); err != nil {
		return fmt.Errorf("set %q: %w", path, err)
	}

	// Write back atomically
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal genesis: %w", err)
	}
	tmp := genesis + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, genesis)
}

// SetGenesisTime sets the genesis_time field in genesis.json.
func SetGenesisTime(timeStr string) error {
	if _, err := time.Parse(time.RFC3339, timeStr); err != nil {
		return fmt.Errorf("invalid time format (expected RFC3339): %w", err)
	}
	return PatchGenesisParam("genesis_time", fmt.Sprintf("%q", timeStr))
}

// setNestedValue traverses a nested map and sets the value at the given key path.
func setNestedValue(doc map[string]any, keys []string, value any) error {
	for _, key := range keys[:len(keys)-1] {
		next, ok := doc[key].(map[string]any)
		if !ok {
			// Create intermediate map if missing
			next = make(map[string]any)
			doc[key] = next
		}
		doc = next
	}
	doc[keys[len(keys)-1]] = value
	return nil
}
