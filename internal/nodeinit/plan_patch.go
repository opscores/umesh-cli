package nodeinit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opscores/umesh-cli/internal/uio"
)


// addBankMetadataFromPlan adds bank denom metadata from plan or defaults.
func addBankMetadataFromPlan(plan *Plan) error {
	denom := plan.Chain.Denom

	// Check if already present
	path := filepath.Join(Home(), "config", "genesis.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read genesis: %w", err)
	}

	var gen GenesisDocument
	if err := json.Unmarshal(data, &gen); err != nil {
		return fmt.Errorf("parse genesis: %w", err)
	}

	if len(gen.AppState.Bank.DenomMetadata) > 0 {
		return nil // Already present
	}

	// Build denom metadata
	display := strings.TrimPrefix(denom, "u")
	metadata := []GenesisDenomMetadata{
		{
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
			URI:     plan.Chain.DenomURI,
		},
	}

	gen.AppState.Bank.DenomMetadata = metadata

	if err := writeGenesis(path, &gen); err != nil {
		return fmt.Errorf("write genesis: %w", err)
	}

	uio.LogInfo("Added denom metadata for %s", denom)
	return nil
}
