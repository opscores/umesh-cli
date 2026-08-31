package role

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opscores/umesh-cli/internal/nodeinfo"
	"github.com/opscores/umesh-cli/internal/uio"
)

var knownRoles = map[string]bool{
	"genesis": true, "validator": true, "sentry": true, "rpc": true,
}

func Validate(role string) error {
	if !knownRoles[role] {
		return fmt.Errorf("unknown role %q: must be one of genesis, validator, sentry, rpc", role)
	}
	return nil
}

func InferFromDir(dataDir string) string {
	base := filepath.Base(dataDir)
	switch {
	case strings.HasSuffix(base, "data-sentry"):
		return "sentry"
	case strings.HasSuffix(base, "data-rpc"):
		return "rpc"
	default:
		return "validator"
	}
}

func Resolve(dataDir, explicitRole string) (string, error) {
	if explicitRole != "" {
		if err := Validate(explicitRole); err != nil {
			return "", err
		}
		return explicitRole, nil
	}

	if info, err := nodeinfo.Read(dataDir); err == nil && info.Mode != "" {
		if err := Validate(info.Mode); err != nil {
			return "", fmt.Errorf("corrupt .node-info: %w", err)
		}
		return info.Mode, nil
	}

	if legacyInferAndMigrate(dataDir) {
		return InferFromDir(dataDir), nil
	}

	uio.LogWarning("could not determine role from .node-info or directory name; defaulting to 'validator'. Re-run with --role <role> to override.")
	return "validator", nil
}

func legacyInferAndMigrate(dataDir string) bool {
	infoPath := nodeinfo.Path(dataDir)

	if _, err := os.Stat(infoPath); err == nil {
		return false
	}

	configPath := filepath.Join(dataDir, "config", "config.toml")
	genesisPath := filepath.Join(dataDir, "config", "genesis.json")
	configExists := false
	if _, err := os.Stat(configPath); err == nil {
		configExists = true
	}
	if _, err := os.Stat(genesisPath); err == nil {
		configExists = true
	}
	if !configExists {
		return false
	}

	inferred := InferFromDir(dataDir)
	if inferred == "validator" {
		uio.LogWarning("Legacy node detected (no .node-info). Could not infer role from directory name. Defaulting to 'validator'. Use --role to override.")
	} else {
		uio.LogWarning("Legacy node detected (no .node-info). Role '%s' inferred from directory name; saved to config/.node-info. Use --role to override if incorrect.", inferred)
	}

	info := nodeinfo.Info{
		Mode:    inferred,
		ChainID: "",
		NodeID:  "",
	}
	if err := nodeinfo.Write(dataDir, info); err != nil {
		uio.LogWarning("Failed to write .node-info for legacy migration: %v", err)
		return false
	}
	return true
}