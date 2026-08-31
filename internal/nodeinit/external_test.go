package nodeinit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opscores/umesh-cli/internal/nodeconfig"
)

func TestSetExternalAddress(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	configDir := filepath.Join(home, "config")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[p2p]\nexternal_address = \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	SetHome(home)

	cases := []struct {
		name string
		addr string
		want string
	}{
		{"bare ip gets default port", "1.2.3.4", "1.2.3.4:26656"},
		{"host:port preserved", "1.2.3.4:26657", "1.2.3.4:26657"},
		{"tcp:// scheme stripped", "tcp://1.2.3.4:26656", "1.2.3.4:26656"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := setExternalAddress(tc.addr); err != nil {
				t.Fatalf("setExternalAddress(%q): %v", tc.addr, err)
			}
			nc, err := nodeconfig.Load(configDir)
			if err != nil {
				t.Fatalf("reload config: %v", err)
			}
			if got := nc.Config.GetString("p2p.external_address", ""); got != tc.want {
				t.Errorf("p2p.external_address = %q, want %q", got, tc.want)
			}
		})
	}

	// Empty address must be a no-op, not an error.
	if err := setExternalAddress(""); err != nil {
		t.Fatalf("setExternalAddress(\"\"): %v", err)
	}
}

func TestJoinHostPortDefault(t *testing.T) {
	cases := []struct {
		host string
		port string
		want string
	}{
		{"1.2.3.4", "", "1.2.3.4:26656"},
		{"1.2.3.4", "26657", "1.2.3.4:26657"},
	}
	for _, tc := range cases {
		if got := joinHostPort(tc.host, tc.port); got != tc.want {
			t.Errorf("joinHostPort(%q,%q) = %q, want %q", tc.host, tc.port, got, tc.want)
		}
	}
}

func TestPlanValidatorExternalAddress(t *testing.T) {
	plan := &Plan{
		Tokenomics: Tokenomics{
			Allocations: []Allocation{
				{Name: "foundation", Type: "base_account"},
				{Name: "validators", Type: "validator_set", Validators: []ValidatorConfig{
					{Name: "v1", ExternalAddress: "9.9.9.9"},
					{Name: "v2", ExternalAddress: "8.8.8.8"},
				}},
			},
		},
	}
	if got := planValidatorExternalAddress(plan); got != "9.9.9.9" {
		t.Errorf("planValidatorExternalAddress = %q, want first validator's address", got)
	}

	empty := &Plan{Tokenomics: Tokenomics{Allocations: []Allocation{{Name: "a", Type: "base_account"}}}}
	if got := planValidatorExternalAddress(empty); got != "" {
		t.Errorf("planValidatorExternalAddress (no validator_set) = %q, want empty", got)
	}
}
