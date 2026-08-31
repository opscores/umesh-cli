package tune

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opscores/umesh-cli/internal/nodeconfig"
)

func TestApply(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configToml := []byte(`
moniker = "test"
[consensus]
timeout_commit = "5s"
[p2p]
pex = true
`)
	appToml := []byte(`
[api]
enable = true
[pruning]
pruning = "default"
`)

	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), configToml, 0o644); err != nil {
		t.Fatalf("failed to write config.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "app.toml"), appToml, 0o644); err != nil {
		t.Fatalf("failed to write app.toml: %v", err)
	}

	// Test validator tuning
	if err := Apply(configDir, RoleValidator, Options{Environment: "test", Moniker: "val1"}); err != nil {
		t.Fatalf("Apply validator failed: %v", err)
	}

	nc, err := nodeconfig.Load(configDir)
	if err != nil {
		t.Fatalf("failed to reload nodeconfig: %v", err)
	}

	if nc.Config.GetString("consensus.timeout_commit", "") != "3s" {
		t.Errorf("expected timeout_commit 3s, got %v", nc.Config.GetString("consensus.timeout_commit", ""))
	}
	if v, _ := nc.Config.Get("p2p.pex"); v != false {
		t.Errorf("expected p2p.pex false for validator, got %v", v)
	}
	if v, _ := nc.App.Get("api.enable"); v != false {
		t.Errorf("expected api.enable false for validator, got %v", v)
	}

	// Test sentry tuning
	if err := Apply(configDir, RoleSentry, Options{Environment: "test", Moniker: "sentry1"}); err != nil {
		t.Fatalf("Apply sentry failed: %v", err)
	}

	nc2, err := nodeconfig.Load(configDir)
	if err != nil {
		t.Fatalf("failed to reload nodeconfig: %v", err)
	}

	if v, _ := nc2.Config.Get("p2p.pex"); v != true {
		t.Errorf("expected p2p.pex true for sentry, got %v", v)
	}
	if v, _ := nc2.App.Get("api.enable"); v != true {
		t.Errorf("expected api.enable true for sentry, got %v", v)
	}
	if v, _ := nc2.App.Get("state-sync.snapshot-interval"); v != int64(5000) {
		t.Errorf("expected snapshot-interval 5000 for sentry, got %v", v)
	}

	// Test RPC tuning with rate limiting
	if err := Apply(configDir, RoleRPC, Options{Environment: "test", Moniker: "rpc1"}); err != nil {
		t.Fatalf("Apply RPC failed: %v", err)
	}

	nc3, err := nodeconfig.Load(configDir)
	if err != nil {
		t.Fatalf("failed to reload nodeconfig: %v", err)
	}

	if v, _ := nc3.Config.Get("rpc.max_open_connections"); v != int64(4000) {
		t.Errorf("expected max_open_connections 4000 for RPC, got %v", v)
	}
	if v, _ := nc3.Config.Get("p2p.max_num_inbound_peers"); v != int64(60) {
		t.Errorf("expected max_num_inbound_peers 60 for RPC, got %v", v)
	}
	if v, _ := nc3.Config.Get("rpc.max_body_bytes"); v != int64(10000000) {
		t.Errorf("expected max_body_bytes 10000000 for RPC, got %v", v)
	}
}

func TestApplyExternalAddress(t *testing.T) {
	tests := []struct {
		role Role
		addr string
		want string
	}{
		{RoleValidator, "1.2.3.4", "1.2.3.4:26656"},
		{RoleSentry, "tcp://5.6.7.8:26656", "5.6.7.8:26656"},
		{RoleRPC, "9.9.9.9", "9.9.9.9:26656"},
		{RoleRPC, "", ""},
	}
	for _, tc := range tests {
		t.Run(string(tc.role)+"_"+tc.addr, func(t *testing.T) {
			configDir := filepath.Join(t.TempDir(), "config")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatalf("failed to create config dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[p2p]\n"), 0o644); err != nil {
				t.Fatalf("failed to write config.toml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "app.toml"), []byte("[api]\n"), 0o644); err != nil {
				t.Fatalf("failed to write app.toml: %v", err)
			}

			if err := Apply(configDir, tc.role, Options{Environment: "test", Moniker: "n", ExternalAddress: tc.addr}); err != nil {
				t.Fatalf("Apply %s failed: %v", tc.role, err)
			}
			nc, err := nodeconfig.Load(configDir)
			if err != nil {
				t.Fatalf("reload config: %v", err)
			}
			got := nc.Config.GetString("p2p.external_address", "")
			if got != tc.want {
				t.Errorf("p2p.external_address = %q, want %q", got, tc.want)
			}
		})
	}
}
