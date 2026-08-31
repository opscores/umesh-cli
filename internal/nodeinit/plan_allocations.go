package nodeinit

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/uio"
)

// createAllocations creates all allocations from the plan.
func createAllocations(d *dkrcmd.Docker, plan *Plan) ([]ResolvedAllocation, error) {
	totalSupply, ok := new(big.Int).SetString(plan.Tokenomics.TotalSupply, 10)
	if !ok {
		return nil, fmt.Errorf("invalid total_supply: %s", plan.Tokenomics.TotalSupply)
	}

	var resolved []ResolvedAllocation
	var totalAllocated big.Int

	for _, alloc := range plan.Tokenomics.Allocations {
		uio.LogStep("Processing allocation: %s (%.2f%%, type=%s)", alloc.Name, alloc.Percentage, alloc.Type)

		amount := percentageToAmount(alloc.Percentage, totalSupply)
		
		switch alloc.Type {
		case "base_account":
			if err := createBaseAccount(d, alloc, amount, plan.Chain.Denom); err != nil {
				return nil, fmt.Errorf("base account %s: %w", alloc.Name, err)
			}

		case "delayed_vesting":
			if err := createDelayedVestingAccount(d, alloc, amount, plan.Chain.Denom); err != nil {
				return nil, fmt.Errorf("delayed vesting %s: %w", alloc.Name, err)
			}

		case "continuous_vesting":
			if err := createContinuousVestingAccount(d, alloc, amount, plan.Chain.Denom); err != nil {
				return nil, fmt.Errorf("continuous vesting %s: %w", alloc.Name, err)
			}

		case "validator_set":
			if err := createValidatorSet(d, alloc, amount, plan.Chain); err != nil {
				return nil, fmt.Errorf("validator set %s: %w", alloc.Name, err)
			}

		default:
			return nil, fmt.Errorf("unknown allocation type: %s", alloc.Type)
		}

		// Resolve address for reporting
		var address string
		var err error
		if alloc.Type == "validator_set" && len(alloc.Validators) > 0 {
			// For validator_set, use the first validator's address
			valAlloc := Allocation{
				Name:    alloc.Validators[0].Name,
				KeyName: alloc.Validators[0].Name,
			}
			address, _, _, err = resolveKey(d, valAlloc)
			if err != nil {
				return nil, fmt.Errorf("resolve address for %s: %w", alloc.Name, err)
			}
		} else {
			address, _, _, err = resolveKey(d, alloc)
			if err != nil {
				return nil, fmt.Errorf("resolve address for %s: %w", alloc.Name, err)
			}
		}

		resolved = append(resolved, ResolvedAllocation{
			Name:       alloc.Name,
			Type:       alloc.Type,
			Percentage: alloc.Percentage,
			Address:    address,
			Amount:     amount,
			Vesting:    alloc.Vesting,
			Validators: alloc.Validators,
		})

		totalAllocated.Add(&totalAllocated, amount)
		uio.LogSuccess("  %s: %s%s -> %s", alloc.Name, amount.String(), plan.Chain.Denom, address)
	}

	// Verify total allocation
	if totalAllocated.Cmp(totalSupply) > 0 {
		return nil, fmt.Errorf("total allocation %s exceeds supply %s", totalAllocated.String(), totalSupply.String())
	}

	return resolved, nil
}

// createBaseAccount creates a base account with the given amount.
func createBaseAccount(d *dkrcmd.Docker, alloc Allocation, amount *big.Int, denom string) error {
	address, _, _, err := resolveKey(d, alloc)
	if err != nil {
		return err
	}

	coinAmount := amount.String() + denom
	_, err = d.RunMount(nil, "genesis", "add-genesis-account", address, coinAmount,
		"--home", containerHome(d))
	if err != nil {
		return fmt.Errorf("add-genesis-account: %w", err)
	}
	return nil
}

// createDelayedVestingAccount creates a delayed vesting account.
func createDelayedVestingAccount(d *dkrcmd.Docker, alloc Allocation, amount *big.Int, denom string) error {
	address, _, _, err := resolveKey(d, alloc)
	if err != nil {
		return err
	}

	endTime, err := parseUnixTime(alloc.Vesting.EndTime)
	if err != nil {
		return fmt.Errorf("invalid end_time: %w", err)
	}

	coinAmount := amount.String() + denom
	_, err = d.RunMount(nil, "genesis", "add-genesis-account", address, coinAmount,
		"--vesting-amount", coinAmount,
		"--vesting-end-time", strconv.FormatInt(endTime, 10),
		"--home", containerHome(d))
	if err != nil {
		return fmt.Errorf("add-genesis-account: %w", err)
	}
	return nil
}

// createContinuousVestingAccount creates a continuous vesting account.
func createContinuousVestingAccount(d *dkrcmd.Docker, alloc Allocation, amount *big.Int, denom string) error {
	address, _, _, err := resolveKey(d, alloc)
	if err != nil {
		return err
	}

	if err := validateVestingTimeRange(alloc.Vesting.EndTime, alloc.Vesting.StartTime); err != nil {
		return fmt.Errorf("vesting time range for %s: %w", alloc.Name, err)
	}

	startTime, err := parseUnixTime(alloc.Vesting.StartTime)
	if err != nil {
		return fmt.Errorf("invalid start_time: %w", err)
	}

	endTime, err := parseUnixTime(alloc.Vesting.EndTime)
	if err != nil {
		return fmt.Errorf("invalid end_time: %w", err)
	}

	coinAmount := amount.String() + denom
	_, err = d.RunMount(nil, "genesis", "add-genesis-account", address, coinAmount,
		"--vesting-amount", coinAmount,
		"--vesting-start-time", strconv.FormatInt(startTime, 10),
		"--vesting-end-time", strconv.FormatInt(endTime, 10),
		"--home", containerHome(d))
	if err != nil {
		return fmt.Errorf("add-genesis-account: %w", err)
	}
	return nil
}

// createValidatorSet creates validators from a validator_set allocation.
func createValidatorSet(d *dkrcmd.Docker, alloc Allocation, totalAmount *big.Int, chain ChainConfig) error {
	if len(alloc.Validators) == 0 {
		return fmt.Errorf("validator_set requires at least one validator")
	}

	// Divide total amount equally among validators
	perValidator := new(big.Int).Div(totalAmount, big.NewInt(int64(len(alloc.Validators))))
	remainder := new(big.Int).Mod(totalAmount, big.NewInt(int64(len(alloc.Validators))))

	for i, val := range alloc.Validators {
		uio.LogInfo("  Creating validator: %s", val.Name)

		// Add remainder to first validator
		validatorAmount := new(big.Int).Set(perValidator)
		if i == 0 {
			validatorAmount.Add(validatorAmount, remainder)
		}

		// Resolve validator key
		valAlloc := Allocation{
			Name:     val.Name,
			KeyName:  val.Name,
			Address:  "",
			Mnemonic: "",
		}
		address, _, _, err := resolveKey(d, valAlloc)
		if err != nil {
			return fmt.Errorf("resolve key for validator %s: %w", val.Name, err)
		}

		// Calculate self-delegation + operational funds
		selfDelegation, ok := new(big.Int).SetString(ExtractAmount(val.SelfDelegation), 10)
		if !ok {
			return fmt.Errorf("invalid self_delegation: %s", val.SelfDelegation)
		}
		operationalFunds, ok := new(big.Int).SetString(ExtractAmount(val.OperationalFunds), 10)
		if !ok {
			return fmt.Errorf("invalid operational_funds: %s", val.OperationalFunds)
		}
		totalValidatorFunds := new(big.Int).Add(selfDelegation, operationalFunds)

		// Add genesis account
		coinAmount := totalValidatorFunds.String() + chain.Denom
		_, err = d.RunMount(nil, "genesis", "add-genesis-account", address, coinAmount,
			"--home", containerHome(d))
		if err != nil {
			return fmt.Errorf("add-genesis-account for %s: %w", val.Name, err)
		}

		// Generate gentx
		if err := generateValidatorGentx(d, val, chain); err != nil {
			return fmt.Errorf("gentx for %s: %w", val.Name, err)
		}
	}

	return nil
}

// generateValidatorGentx generates gentx for a single validator.
func generateValidatorGentx(d *dkrcmd.Docker, val ValidatorConfig, chain ChainConfig) error {
	amountStr := ExtractAmount(val.SelfDelegation)
	amount, ok := new(big.Int).SetString(amountStr, 10)
	if !ok || amount.Sign() <= 0 {
		return fmt.Errorf("self_delegation must be positive")
	}

	gentxDir := containerHome(d) + "/config/gentx"
	if ForceReinit {
		_, _ = d.RunMount(nil, "sh", "-c", "rm -f "+gentxDir+"/gentx-*.json")
	}

	gentxAmount := amount.String() + chain.Denom
	args := []string{"genesis", "gentx", val.Name,
		gentxAmount,
		"--chain-id", chain.ChainID,
		"--keyring-backend", "file",
		"--keyring-dir", containerHome(d) + "/keyring",
		"--home", containerHome(d),
		"--moniker", chain.Moniker,
	}

	if val.ExternalAddress != "" {
		args = append(args, "--ip", val.ExternalAddress)
	}
	if val.Identity != "" {
		args = append(args, "--identity", val.Identity)
	}
	if val.Website != "" {
		args = append(args, "--website", val.Website)
	}
	if val.SecurityContact != "" {
		args = append(args, "--security-contact", val.SecurityContact)
	}
	if val.Details != "" {
		args = append(args, "--details", val.Details)
	}
	if val.CommissionRate != "" {
		args = append(args, "--commission-rate", val.CommissionRate)
	}
	if val.CommissionMax != "" {
		args = append(args, "--commission-max-rate", val.CommissionMax)
	}
	if val.CommissionMaxChange != "" {
		args = append(args, "--commission-max-change-rate", val.CommissionMaxChange)
	}
	if val.MinSelfDelegation != "" {
		args = append(args, "--min-self-delegation", val.MinSelfDelegation)
	}

	_, err := d.RunMount(strings.NewReader(keyringPassword+"\n"), args...)
	if err != nil {
		return fmt.Errorf("gentx: %w", err)
	}

	return nil
}

// ResolveKeyPublic is the exported version of resolveKey for use by CLI commands.
func ResolveKeyPublic(d *dkrcmd.Docker, alloc Allocation) (address string, generated bool, mnemonic string, err error) {
	return resolveKey(d, alloc)
}

// addSingleBaseAccount adds a single base account.
func AddSingleBaseAccount(d *dkrcmd.Docker, alloc Allocation, amount *big.Int, denom string) error {
	address, _, _, err := resolveKey(d, alloc)
	if err != nil {
		return err
	}

	coinAmount := amount.String() + denom
	_, err = d.RunMount(nil, "genesis", "add-genesis-account", address, coinAmount,
		"--home", containerHome(d))
	return err
}

// addSingleDelayedVesting adds a single delayed vesting account.
func AddSingleDelayedVesting(d *dkrcmd.Docker, alloc Allocation, amount *big.Int, denom string) error {
	address, _, _, err := resolveKey(d, alloc)
	if err != nil {
		return err
	}

	if alloc.Vesting == nil || alloc.Vesting.EndTime == "" {
		return fmt.Errorf("delayed vesting requires --end-time")
	}
	endTime, err := parseUnixTime(alloc.Vesting.EndTime)
	if err != nil {
		return fmt.Errorf("invalid end_time: %w", err)
	}

	coinAmount := amount.String() + denom
	_, err = d.RunMount(nil, "genesis", "add-genesis-account", address, coinAmount,
		"--vesting-amount", coinAmount,
		"--vesting-end-time", fmt.Sprintf("%d", endTime),
		"--home", containerHome(d))
	return err
}

// addSingleContinuousVesting adds a single continuous vesting account.
func AddSingleContinuousVesting(d *dkrcmd.Docker, alloc Allocation, amount *big.Int, denom string) error {
	address, _, _, err := resolveKey(d, alloc)
	if err != nil {
		return err
	}

	if alloc.Vesting == nil || alloc.Vesting.StartTime == "" || alloc.Vesting.EndTime == "" {
		return fmt.Errorf("continuous vesting requires --start-time and --end-time")
	}
	startTime, err := parseUnixTime(alloc.Vesting.StartTime)
	if err != nil {
		return fmt.Errorf("invalid start_time: %w", err)
	}
	endTime, err := parseUnixTime(alloc.Vesting.EndTime)
	if err != nil {
		return fmt.Errorf("invalid end_time: %w", err)
	}

	coinAmount := amount.String() + denom
	_, err = d.RunMount(nil, "genesis", "add-genesis-account", address, coinAmount,
		"--vesting-amount", coinAmount,
		"--vesting-start-time", fmt.Sprintf("%d", startTime),
		"--vesting-end-time", fmt.Sprintf("%d", endTime),
		"--home", containerHome(d))
	return err
}

// AddSingleModuleAccount adds a single module account using the --module-name flag.
// Vanilla Cosmos SDK 0.54's add-genesis-account supports --module-name for creating
// module accounts.
func AddSingleModuleAccount(d *dkrcmd.Docker, alloc Allocation, amount *big.Int, denom string) error {
	address, _, _, err := resolveKey(d, alloc)
	if err != nil {
		return err
	}

	if alloc.ModuleName == "" {
		return fmt.Errorf("module_name is required for module_account")
	}

	coinAmount := amount.String() + denom
	_, err = d.RunMount(nil, "genesis", "add-genesis-account", address, coinAmount,
		"--module-name", alloc.ModuleName,
		"--home", containerHome(d))
	if err != nil {
		return fmt.Errorf("add-genesis-account: %w", err)
	}
	return nil
}
