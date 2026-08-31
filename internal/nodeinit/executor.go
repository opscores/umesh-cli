package nodeinit

import (
	"fmt"
)

// ExecFromPlan parses and executes a genesis plan for production genesis.
// It is used by `genesis plan --config <file>`.
// If dryRun is true, the plan is validated and a summary printed but not executed.
func ExecFromPlan(planPath string, dryRun bool) error {
	plan, err := ParsePlan(planPath)
	if err != nil {
		return fmt.Errorf("parse plan: %w", err)
	}
	if err := ValidatePlan(plan); err != nil {
		return fmt.Errorf("validate plan: %w", err)
	}

	if err := ExecutePlanFromEnv(plan, GetKeyringPass(), dryRun); err != nil {
		return fmt.Errorf("execute plan: %w", err)
	}
	return nil
}
