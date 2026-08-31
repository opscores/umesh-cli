package nodeinit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteOtelConfigOptIn(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	SetHome(home)

	if err := WriteOtelConfig("umesh-validator", "http://otel:4317", "production"); err != nil {
		t.Fatalf("WriteOtelConfig: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(ConfigDir(), "otel.yaml"))
	if err != nil {
		t.Fatalf("read otel.yaml: %v", err)
	}
	got := string(b)
	for _, want := range []string{
		`file_format: "1.0.0"`,
		"service.name",
		"umesh-validator",
		"deployment.environment",
		"production",
		"http://otel:4317",
		"otlp_grpc",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("otel.yaml missing %q:\n%s", want, got)
		}
	}
}

func TestWriteOtelConfigOptOut(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	SetHome(home)
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ConfigDir(), "otel.yaml"), []byte("empty"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Empty endpoint must leave the existing file untouched.
	if err := WriteOtelConfig("umesh-validator", "", "production"); err != nil {
		t.Fatalf("WriteOtelConfig: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(ConfigDir(), "otel.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "empty" {
		t.Errorf("otel.yaml was modified for empty endpoint: %q", string(b))
	}
}