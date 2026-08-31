package nodeinit

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/opscores/umesh-cli/internal/uio"
)

// handleSoftLaunch configures soft launch parameters.
func handleSoftLaunch(plan *Plan) error {
	if !plan.SoftLaunch.Enabled {
		return nil
	}

	enabled, err := enabledAppStateModules()
	if err != nil {
		return err
	}

	denom := plan.Chain.Denom

	// Disable bank send for the native denom
	if plan.SoftLaunch.DisableBankSend {
		if err := patchModuleParamsIfEnabled("bank", enabled, func() error {
			uio.LogInfo("Disabling bank send for %s", denom)

			// Set send_enabled to disable native denom transfers. In SDK >= 0.47
			// send_enabled lives at the top level of the bank genesis state, NOT in
			// bank.params (that location is deprecated and rejected by validation).
			sendEnabled := []map[string]any{
				{"denom": denom, "enabled": false},
			}
			data, err := json.Marshal(sendEnabled)
			if err != nil {
				return fmt.Errorf("marshal send_enabled: %w", err)
			}
			if err := PatchGenesisParam("app_state.bank.send_enabled", string(data)); err != nil {
				return fmt.Errorf("patch send_enabled: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Disable IBC transfer
	if plan.SoftLaunch.DisableIBCTransfer {
		if err := patchModuleParamsIfEnabled("transfer", enabled, func() error {
			uio.LogInfo("Disabling IBC transfers")

			// Disable send for IBC transfer
			if err := PatchGenesisParam("app_state.transfer.params.send_enabled", "false"); err != nil {
				return err
			}
			if err := PatchGenesisParam("app_state.transfer.params.receive_enabled", "false"); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Wasm contract instantiation is controllable via a real genesis param.
	// Only act when the field is explicitly set in the plan (nil = unset).
	if plan.SoftLaunch.AllowWasmInstantiate != nil {
		if err := patchModuleParamsIfEnabled("wasm", enabled, func() error {
			permission := "Nobody"
			if *plan.SoftLaunch.AllowWasmInstantiate {
				permission = "Everybody"
			}
			if err := PatchGenesisParam("app_state.wasm.params.instantiate_default_permission", fmt.Sprintf("%q", permission)); err != nil {
				return err
			}
			uio.LogInfo("Wasm instantiate permission set to %s", permission)
			return nil
		}); err != nil {
			return err
		}
	}

	// Staking and governance have no corresponding genesis parameter to toggle;
	// these fields are accepted for forward-compatibility only.
	if plan.SoftLaunch.AllowStaking != nil && !*plan.SoftLaunch.AllowStaking {
		uio.LogWarning("soft_launch.allow_staking is reserved and not applied: staking must stay enabled for consensus")
	}
	if plan.SoftLaunch.AllowGov != nil && !*plan.SoftLaunch.AllowGov {
		uio.LogWarning("soft_launch.allow_gov is reserved and not applied yet")
	}

	// Disable inflation for the soft launch period
	if plan.SoftLaunch.DisableInflation {
		if err := patchModuleParamsIfEnabled("mint", enabled, func() error {
			uio.LogInfo("Disabling inflation for soft launch")

			if err := PatchGenesisParam("app_state.mint.params.inflation_min", `"0.000000000000000000"`); err != nil {
				return err
			}
			if err := PatchGenesisParam("app_state.mint.params.inflation_max", `"0.000000000000000000"`); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// handleDust calculates and allocates dust (remainder from rounding) to its destination.
func handleDust(plan *Plan, resolved []ResolvedAllocation) error {
	totalSupply, ok := new(big.Int).SetString(plan.Tokenomics.TotalSupply, 10)
	if !ok {
		return fmt.Errorf("invalid total_supply: %s", plan.Tokenomics.TotalSupply)
	}

	// Calculate total allocated
	var totalAllocated big.Int
	for _, alloc := range plan.Tokenomics.Allocations {
		amount := percentageToAmount(alloc.Percentage, totalSupply)
		totalAllocated.Add(&totalAllocated, amount)
	}

	// Calculate dust
	dust := new(big.Int).Sub(totalSupply, &totalAllocated)
	if dust.Sign() <= 0 {
		return nil
	}

	uio.LogInfo("Dust amount: %s %s", dust.String(), plan.Chain.Denom)

	// Determine dust destination
	destination := plan.Tokenomics.Validation.DustDestination
	if destination == "" {
		destination = "community_pool"
	}

	switch destination {
	case "community_pool":
		return allocateDustToCommunityPool(dust, plan.Chain.Denom)
	case "foundation":
		return allocateDustToFoundation(dust, plan.Chain.Denom, resolved)
	default:
		uio.LogWarning("Unknown dust destination %s, using community_pool", destination)
		return allocateDustToCommunityPool(dust, plan.Chain.Denom)
	}
}

// allocateDustToCommunityPool sends dust to the community pool.
func allocateDustToCommunityPool(dust *big.Int, denom string) error {
	enabled, err := enabledAppStateModules()
	if err != nil {
		return err
	}
	if !enabled["distribution"] {
		uio.LogWarning("module %q is not compiled into umeshd; skipping dust to community pool", "distribution")
		return nil
	}

	// Add to distribution.fee_pool.community_pool
	communityPoolCoins := []map[string]any{
		{"denom": denom, "amount": dust.String()},
	}
	data, err := json.Marshal(communityPoolCoins)
	if err != nil {
		return fmt.Errorf("marshal community pool coins: %w", err)
	}
	if err := PatchGenesisParam("app_state.distribution.fee_pool.community_pool", string(data)); err != nil {
		return fmt.Errorf("patch community pool: %w", err)
	}
	uio.LogSuccess("Allocated dust %s%s to community pool", dust.String(), denom)
	return nil
}

// allocateDustToFoundation sends dust to the foundation account's genesis balance.
func allocateDustToFoundation(dust *big.Int, denom string, resolved []ResolvedAllocation) error {
	// Find the foundation allocation's resolved address.
	var addr string
	for _, r := range resolved {
		if strings.EqualFold(r.Name, "foundation") {
			addr = r.Address
			break
		}
	}
	if addr == "" {
		uio.LogWarning("Foundation allocation not found in plan, sending dust to community pool")
		return allocateDustToCommunityPool(dust, denom)
	}

	genesisPath := filepath.Join(Home(), "config", "genesis.json")
	data, err := os.ReadFile(genesisPath)
	if err != nil {
		return fmt.Errorf("read genesis: %w", err)
	}

	var gen map[string]any
	if err := json.Unmarshal(data, &gen); err != nil {
		return fmt.Errorf("parse genesis: %w", err)
	}

	appState, ok := gen["app_state"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid app_state")
	}
	bank, ok := appState["bank"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid bank module")
	}
	balances, ok := bank["balances"].([]any)
	if !ok {
		return fmt.Errorf("invalid bank balances")
	}

	updated := false
	for _, b := range balances {
		entry, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if a, ok := entry["address"].(string); !ok || a != addr {
			continue
		}

		coins, _ := entry["coins"].([]any)
		found := false
		for _, c := range coins {
			coin, ok := c.(map[string]any)
			if !ok || coin["denom"] != denom {
				continue
			}
			amount, ok := new(big.Int).SetString(fmt.Sprintf("%v", coin["amount"]), 10)
			if !ok {
				return fmt.Errorf("invalid balance amount for %s: %v", addr, coin["amount"])
			}
			coin["amount"] = new(big.Int).Add(amount, dust).String()
			found = true
			break
		}
		if !found {
			entry["coins"] = append(coins, map[string]any{"denom": denom, "amount": dust.String()})
		}
		updated = true
		break
	}
	if !updated {
		uio.LogWarning("Foundation balance for %s not found in genesis, sending dust to community pool", addr)
		return allocateDustToCommunityPool(dust, denom)
	}

	bank["balances"] = balances
	appState["bank"] = bank
	gen["app_state"] = appState

	out, err := json.MarshalIndent(gen, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal genesis: %w", err)
	}
	tmp := genesisPath + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("write genesis temp: %w", err)
	}
	if err := os.Rename(tmp, genesisPath); err != nil {
		return fmt.Errorf("rename genesis: %w", err)
	}

	uio.LogSuccess("Allocated dust %s%s to foundation (%s)", dust.String(), denom, addr)
	return nil
}
