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

// generateReport generates a report of the genesis plan execution.
func generateReport(plan *Plan, resolved []ResolvedAllocation) error {
	if len(resolved) == 0 {
		return nil
	}

	denom := plan.Chain.Denom
	totalSupply := plan.Tokenomics.TotalSupply

	uio.Print("")
	uio.Print("========================================")
	uio.Print("  GENESIS PLAN REPORT")
	uio.Print("========================================")
	uio.Print("")
	uio.Print("Chain ID:    %s", plan.Chain.ChainID)
	uio.Print("Denom:       %s", denom)
	uio.Print("Total Supply: %s %s", totalSupply, denom)
	genBytes, _ := os.ReadFile(GenesisFile())
	genSHA := SHA256Hex(genBytes)
	uio.Print("Genesis SHA: %s", genSHA)
	uio.Print("")
	uio.Print("Allocations:")
	uio.Print("%-20s %-20s %10s %25s", "Name", "Type", "%", "Address")
	uio.Print("%s", strings.Repeat("-", 80))

	var totalPercentage float64

	for _, r := range resolved {
		uio.Print("%-20s %-20s %9.2f%% %25s", r.Name, r.Type, r.Percentage, r.Address)
		totalPercentage += r.Percentage
	}

	uio.Print("%s", strings.Repeat("-", 80))
	uio.Print("%-40s %9.2f%%", "TOTAL", totalPercentage)
	uio.Print("")

	// Print vesting details
	hasVesting := false
	for _, r := range resolved {
		if r.Vesting != nil {
			if !hasVesting {
				uio.Print("Vesting Details:")
				hasVesting = true
			}
			uio.Print("  %s:", r.Name)
			if r.Vesting.StartTime != "" {
				uio.Print("    Start: %s", r.Vesting.StartTime)
			}
			if r.Vesting.EndTime != "" {
				uio.Print("    End:   %s", r.Vesting.EndTime)
			}
			if r.Vesting.CliffDuration != "" {
				uio.Print("    Cliff: %s", r.Vesting.CliffDuration)
			}
		}
	}

	// Print validator details
	hasValidators := false
	for _, r := range resolved {
		if r.Type == "validator_set" && len(r.Validators) > 0 {
			if !hasValidators {
				uio.Print("")
				uio.Print("Validators:")
				hasValidators = true
			}
			for _, v := range r.Validators {
				uio.Print("  %s:", v.Name)
				uio.Print("    Self-delegation: %s%s", v.SelfDelegation, denom)
				uio.Print("    Commission rate: %s", v.CommissionRate)
			}
		}
	}

	// Print soft launch status
	if plan.SoftLaunch.Enabled {
		uio.Print("")
		uio.Print("Soft Launch: ENABLED")
		if plan.SoftLaunch.DisableBankSend {
			uio.Print("  - Bank send for %s: DISABLED", denom)
		}
		if plan.SoftLaunch.DisableIBCTransfer {
			uio.Print("  - IBC transfers: DISABLED")
		}
		if plan.SoftLaunch.AllowStaking != nil && *plan.SoftLaunch.AllowStaking {
			uio.Print("  - Staking: ENABLED")
		}
		if plan.SoftLaunch.AllowGov != nil && *plan.SoftLaunch.AllowGov {
			uio.Print("  - Governance: ENABLED")
		}
	}

	// Print warnings
	uio.Print("")
	uio.Print("Warnings:")
	v := plan.Tokenomics.Validation
	insiderPct := 0.0
	for _, r := range resolved {
		if IsInsider(r.Name) {
			insiderPct += r.Percentage
		}
	}
	if insiderPct > v.MaxInsiderAllocation*0.9 {
		uio.LogWarning("  Insider allocation (%.1f%%) approaching limit (%.1f%%)", insiderPct, v.MaxInsiderAllocation)
	}
	if totalPercentage < 99.99 {
		uio.LogWarning("  Total allocation %.2f%% < 100%% (dust will go to community pool)", totalPercentage)
	}

	uio.Print("")
	uio.Print("========================================")

	// Write report to file
	reportPath := filepath.Join(Home(), "config", "genesis-plan-report.txt")
	var sb strings.Builder
	fmt.Fprintf(&sb, "Genesis Plan Report for %s\n", plan.Chain.ChainID)
	fmt.Fprintf(&sb, "Total Supply: %s %s\n", totalSupply, denom)
	fmt.Fprintf(&sb, "Genesis SHA256: %s\n", genSHA)
	fmt.Fprintf(&sb, "Allocations: %d\n\n", len(resolved))
	for _, r := range resolved {
		fmt.Fprintf(&sb, "- %s (%s): %.2f%% -> %s\n", r.Name, r.Type, r.Percentage, r.Address)
	}
	if err := os.WriteFile(reportPath, []byte(sb.String()), 0o644); err != nil {
		uio.LogWarning("Could not write report to %s: %v", reportPath, err)
	} else {
		uio.LogInfo("Report written to %s", reportPath)
	}

	return nil
}

// PrintPlanReport prints a report from a plan without executing it.
func PrintPlanReport(plan *Plan, format string) error {
	totalSupply, ok := new(big.Int).SetString(plan.Tokenomics.TotalSupply, 10)
	if !ok {
		return fmt.Errorf("invalid total_supply: %s", plan.Tokenomics.TotalSupply)
	}

	denom := plan.Chain.Denom

	if format == "json" {
		type allocationJSON struct {
			Name       string  `json:"name"`
			Type       string  `json:"type"`
			Percentage float64 `json:"percentage"`
			Amount     string  `json:"amount"`
		}
		type consensusJSON struct {
			BlockMaxBytes           int64    `json:"block_max_bytes"`
			BlockMaxGas             int64    `json:"block_max_gas"`
			TimeIotaMs              *int64   `json:"time_iota_ms,omitempty"`
			EvidenceMaxAgeNumBlocks int64    `json:"evidence_max_age_num_blocks"`
			EvidenceMaxAgeDuration  string   `json:"evidence_max_age_duration"`
			EvidenceMaxBytes        int64    `json:"evidence_max_bytes"`
			ValidatorPubKeyTypes    []string `json:"validator_pub_key_types"`
		}
		type reportJSON struct {
			ChainID     string           `json:"chain_id"`
			Denom       string           `json:"denom"`
			TotalSupply string           `json:"total_supply"`
			Consensus   consensusJSON    `json:"consensus_params"`
			Allocations []allocationJSON `json:"allocations"`
		}

		report := reportJSON{
			ChainID:     plan.Chain.ChainID,
			Denom:       denom,
			TotalSupply: totalSupply.String(),
		}
		def := resolveConsensusParams(plan.Chain.Consensus)
		report.Consensus = consensusJSON{
			BlockMaxBytes:           def.BlockMaxBytes,
			BlockMaxGas:             def.BlockMaxGas,
			TimeIotaMs:              resolveTimeIotaPtr(plan.Chain.Consensus, def),
			EvidenceMaxAgeNumBlocks: def.EvidenceMaxAgeNumBlocks,
			EvidenceMaxAgeDuration:  def.EvidenceMaxAgeDuration,
			EvidenceMaxBytes:        def.EvidenceMaxBytes,
			ValidatorPubKeyTypes:    def.ValidatorPubKeyTypes,
		}

		for _, alloc := range plan.Tokenomics.Allocations {
			amount := percentageToAmount(alloc.Percentage, totalSupply)
			report.Allocations = append(report.Allocations, allocationJSON{
				Name:       alloc.Name,
				Type:       alloc.Type,
				Percentage: alloc.Percentage,
				Amount:     amount.String() + " " + denom,
			})
		}

		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		uio.Print(string(data))
		return nil
	}

	// Default: table format
	uio.Print("")
	uio.Print("========================================")
	uio.Print("  GENESIS PLAN REPORT (preview)")
	uio.Print("========================================")
	uio.Print("")
	uio.Print("Chain ID:     %s", plan.Chain.ChainID)
	uio.Print("Denom:        %s", denom)
	uio.Print("Total Supply: %s %s", totalSupply.String(), denom)
	uio.Print("")
	uio.Print("Consensus Params:")
	d := resolveConsensusParams(plan.Chain.Consensus)
	uio.Print("  block.max_bytes:            %d", d.BlockMaxBytes)
	uio.Print("  block.max_gas:              %d", d.BlockMaxGas)
	// time_iota_ms is only written by legacy CometBFT layouts; show it only
	// when the plan explicitly sets it so the preview matches the genesis.
	if plan.Chain.Consensus.TimeIotaMs != 0 {
		uio.Print("  block.time_iota_ms:         %d", d.TimeIotaMs)
	}
	uio.Print("  evidence.max_age_num_blocks: %d", d.EvidenceMaxAgeNumBlocks)
	uio.Print("  evidence.max_age_duration:   %s", d.EvidenceMaxAgeDuration)
	uio.Print("  evidence.max_bytes:          %d", d.EvidenceMaxBytes)
	uio.Print("  validator.pub_key_types:     %v", d.ValidatorPubKeyTypes)
	uio.Print("")
	uio.Print("%-20s %-20s %10s %25s", "Name", "Type", "%", "Amount")
	uio.Print("%s", strings.Repeat("-", 80))

	var totalPercentage float64
	for _, alloc := range plan.Tokenomics.Allocations {
		amount := percentageToAmount(alloc.Percentage, totalSupply)
		uio.Print("%-20s %-20s %9.2f%% %25s", alloc.Name, alloc.Type, alloc.Percentage, amount.String()+" "+denom)
		totalPercentage += alloc.Percentage
	}

	uio.Print("%s", strings.Repeat("-", 80))
	uio.Print("%-40s %9.2f%%", "TOTAL", totalPercentage)
	uio.Print("")

	// Print vesting details in preview report if any exist
	hasVesting := false
	for _, alloc := range plan.Tokenomics.Allocations {
		if alloc.Vesting != nil {
			if !hasVesting {
				uio.Print("Vesting Details:")
				hasVesting = true
			}
			uio.Print("  %s:", alloc.Name)
			if alloc.Vesting.StartTime != "" {
				uio.Print("    Start: %s", alloc.Vesting.StartTime)
			}
			if alloc.Vesting.EndTime != "" {
				uio.Print("    End:   %s", alloc.Vesting.EndTime)
			}
			if alloc.Vesting.CliffDuration != "" {
				uio.Print("    Cliff: %s", alloc.Vesting.CliffDuration)
			}
		}
	}

	// Print validator commission summary in preview report
	PrintGentxCommissionSummary(plan)

	uio.Print("========================================")

	return nil
}

// PrintGentxCommissionSummary prints a summary table of validators and their commission parameters after gentx collection.
func PrintGentxCommissionSummary(plan *Plan) {
	hasValidators := false
	for _, alloc := range plan.Tokenomics.Allocations {
		if alloc.Type == "validator_set" && len(alloc.Validators) > 0 {
			if !hasValidators {
				uio.Print("")
				uio.Print("========================================")
				uio.Print("  VALIDATOR GENTX & COMMISSION SUMMARY")
				uio.Print("========================================")
				uio.Print("%-15s %-15s %-12s %-12s %-12s", "Validator", "Self-Delegation", "Commission", "Max Rate", "Max Change")
				uio.Print("%s", strings.Repeat("-", 75))
				hasValidators = true
			}
			for _, v := range alloc.Validators {
				rate := v.CommissionRate
				if rate == "" {
					rate = "0.10 (default)"
				}
				maxRate := v.CommissionMax
				if maxRate == "" {
					maxRate = "0.20 (default)"
				}
				maxChange := v.CommissionMaxChange
				if maxChange == "" {
					maxChange = "0.01 (default)"
				}
				uio.Print("%-15s %-15s %-12s %-12s %-12s", v.Name, v.SelfDelegation, rate, maxRate, maxChange)
			}
		}
	}
	if hasValidators {
		uio.Print("%s", strings.Repeat("-", 75))
		uio.Print("")
	}
}
