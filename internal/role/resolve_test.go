package role

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opscores/umesh-cli/internal/nodeinfo"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		role     string
		wantErr  bool
	}{
		{"genesis", false},
		{"validator", false},
		{"sentry", false},
		{"rpc", false},
		{"join", true},
		{"", true},
		{"unknown", true},
		{"banana", true},
	}
	for _, tc := range tests {
		err := Validate(tc.role)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Validate(%q): expected error, got nil", tc.role)
			}
		} else {
			if err != nil {
				t.Errorf("Validate(%q): unexpected error: %v", tc.role, err)
			}
		}
	}
}

func TestInferFromDir(t *testing.T) {
	tests := []struct {
		dir      string
		expected string
	}{
		{"/path/to/data-sentry", "sentry"},
		{"./data-sentry", "sentry"},
		{"/path/to/data-rpc", "rpc"},
		{"./data-rpc", "rpc"},
		{"/path/to/data-validator", "validator"},
		{"./data-validator", "validator"},
		{"/path/to/data-genesis", "validator"},
		{"./data-genesis", "validator"},
		{"/path/to/data-xyz", "validator"},
		{"./data-xyz", "validator"},
	}
	for _, tc := range tests {
		got := InferFromDir(tc.dir)
		if got != tc.expected {
			t.Errorf("InferFromDir(%q) = %q, want %q", tc.dir, got, tc.expected)
		}
	}
}

func TestResolve_ExplicitOverrideWins(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-sentry")
	if err := os.MkdirAll(filepath.Join(dataDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = nodeinfo.Write(dataDir, nodeinfo.Info{Mode: "sentry", ChainID: "test"})

	got, err := Resolve(dataDir, "rpc")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != "rpc" {
		t.Errorf("expected 'rpc', got %q", got)
	}
}

func TestResolve_FromNodeInfo(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-rpc")
	if err := os.MkdirAll(filepath.Join(dataDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = nodeinfo.Write(dataDir, nodeinfo.Info{Mode: "rpc", ChainID: "test"})

	got, err := Resolve(dataDir, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != "rpc" {
		t.Errorf("expected 'rpc', got %q", got)
	}
}

func TestResolve_Legacy_InferSentry(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-sentry")
	configDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(dataDir, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != "sentry" {
		t.Errorf("expected 'sentry', got %q", got)
	}

	infoPath := nodeinfo.Path(dataDir)
	if _, err := os.Stat(infoPath); os.IsNotExist(err) {
		t.Error(".node-info should have been created by legacy migration")
	}
	info, err := nodeinfo.Read(dataDir)
	if err != nil {
		t.Fatalf("failed to read migrated .node-info: %v", err)
	}
	if info.Mode != "sentry" {
		t.Errorf("migrated .node-info Mode = %q, want 'sentry'", info.Mode)
	}
}

func TestResolve_Legacy_InferRPC(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-rpc")
	configDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "genesis.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(dataDir, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != "rpc" {
		t.Errorf("expected 'rpc', got %q", got)
	}

	infoPath := nodeinfo.Path(dataDir)
	if _, err := os.Stat(infoPath); os.IsNotExist(err) {
		t.Error(".node-info should have been created by legacy migration")
	}
}

func TestResolve_Legacy_DefaultValidator(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-validator")
	configDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(dataDir, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != "validator" {
		t.Errorf("expected 'validator', got %q", got)
	}
}

func TestResolve_NoInitializedDir(t *testing.T) {
	dir := t.TempDir()
	// No config.toml, no genesis.json, no .node-info
	got, err := Resolve(dir, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != "validator" {
		t.Errorf("expected 'validator', got %q", got)
	}
}

func TestResolve_InvalidExplicitRole(t *testing.T) {
	dir := t.TempDir()
	_, err := Resolve(dir, "banana")
	if err == nil {
		t.Error("expected error for invalid role 'banana'")
	}
	if !strings.Contains(err.Error(), "unknown role") {
		t.Errorf("error message should mention unknown role: %v", err)
	}
}

func TestResolve_CorruptNodeInfo(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data-validator")
	if err := os.MkdirAll(filepath.Join(dataDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = nodeinfo.Write(dataDir, nodeinfo.Info{Mode: "garbage", ChainID: "test"})

	_, err := Resolve(dataDir, "")
	if err == nil {
		t.Error("expected error for corrupt .node-info Mode")
	}
	if !strings.Contains(err.Error(), "corrupt") {
		t.Errorf("error message should mention corrupt: %v", err)
	}
}