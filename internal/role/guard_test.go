package role

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/nodeinfo"
)

func TestCheck_AllowedRole(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-validator")
	if err := os.MkdirAll(filepath.Join(dataDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = nodeinfo.Write(dataDir, nodeinfo.Info{Mode: "validator", ChainID: "test"})

	cmd := &cobra.Command{}
	cmd.Flags().String("data-dir", dataDir, "")
	cmd.Flags().String("role", "", "")

	role, err := Check(cmd, []string{"validator", "genesis"})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if role != "validator" {
		t.Errorf("expected 'validator', got %q", role)
	}
}

func TestCheck_ForbiddenRole(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-sentry")
	if err := os.MkdirAll(filepath.Join(dataDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = nodeinfo.Write(dataDir, nodeinfo.Info{Mode: "sentry", ChainID: "test"})

	cmd := &cobra.Command{}
	cmd.Flags().String("data-dir", dataDir, "")
	cmd.Flags().String("role", "", "")

	_, err := Check(cmd, []string{"validator", "genesis"})
	if err == nil {
		t.Error("expected error for forbidden role")
	}
	if !strings.Contains(err.Error(), "not available for role") {
		t.Errorf("error message should mention forbidden role: %v", err)
	}
}

func TestCheck_ExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-sentry")
	if err := os.MkdirAll(filepath.Join(dataDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = nodeinfo.Write(dataDir, nodeinfo.Info{Mode: "sentry", ChainID: "test"})

	cmd := &cobra.Command{}
	cmd.Flags().String("data-dir", dataDir, "")
	cmd.Flags().String("role", "rpc", "") // explicit override

	role, err := Check(cmd, []string{"rpc"})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if role != "rpc" {
		t.Errorf("expected 'rpc' from override, got %q", role)
	}
}

func TestCheck_DataDirOverride(t *testing.T) {
	dir := t.TempDir()
	dataDir1 := filepath.Join(dir, "data-validator")
	dataDir2 := filepath.Join(dir, "data-rpc")
	for _, d := range []string{dataDir1, dataDir2} {
		if err := os.MkdirAll(filepath.Join(d, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = nodeinfo.Write(dataDir1, nodeinfo.Info{Mode: "validator", ChainID: "test"})
	_ = nodeinfo.Write(dataDir2, nodeinfo.Info{Mode: "rpc", ChainID: "test"})

	cmd := &cobra.Command{}
	cmd.Flags().String("data-dir", dataDir2, "") // override to rpc dir
	cmd.Flags().String("role", "", "")

	role, err := Check(cmd, []string{"rpc"})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if role != "rpc" {
		t.Errorf("expected 'rpc' from data-dir override, got %q", role)
	}
}

func TestCheck_LegacyAutoMigration(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-sentry")
	configDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	// No .node-info - legacy node

	cmd := &cobra.Command{}
	cmd.Flags().String("data-dir", dataDir, "")
	cmd.Flags().String("role", "", "")

	role, err := Check(cmd, []string{"sentry", "validator"})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if role != "sentry" {
		t.Errorf("expected 'sentry' from legacy migration, got %q", role)
	}
}