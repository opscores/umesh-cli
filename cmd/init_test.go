package cmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/yamlconfig"
)

// captureOutput redirects os.Stdout and os.Stderr during fn and returns their
// captured contents.
func captureOutput(fn func()) (stdout, stderr string) {
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wo, we

	fn()

	_ = wo.Close()
	_ = we.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	var bufOut, bufErr bytes.Buffer
	_, _ = bufOut.ReadFrom(ro)
	_, _ = bufErr.ReadFrom(re)
	return bufOut.String(), bufErr.String()
}

func TestRunInitAlreadyInitialized(t *testing.T) {
	stdout, stderr := captureOutput(func() {
		err := runInit(nodeinit.ErrAlreadyInitialized)
		if err != nil {
			t.Errorf("runInit(ErrAlreadyInitialized) returned err: %v", err)
		}
	})
	out := stdout + stderr
	if !strings.Contains(out, "already initialized") {
		t.Errorf("expected warning mentioning 'already initialized', got: %q", out)
	}
	if !strings.Contains(out, "init <role> --force") {
		t.Errorf("expected hint about init <role> --force, got: %q", out)
	}
}

func TestRunInitRealError(t *testing.T) {
	sentinel := errors.New("some real failure")
	stdout, stderr := captureOutput(func() {
		err := runInit(sentinel)
		if err == nil {
			t.Error("runInit(real error) returned nil, want err")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("runInit() wrapped error = %v, want the original sentinel", err)
		}
	})
	if stdout != "" || stderr != "" {
		t.Errorf("runInit(real error) should produce no output, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestRunInitNil(t *testing.T) {
	stdout, stderr := captureOutput(func() {
		if err := runInit(nil); err != nil {
			t.Errorf("runInit(nil) returned err: %v", err)
		}
	})
	if stdout != "" || stderr != "" {
		t.Errorf("runInit(nil) should produce no output, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestNormalizeRole(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"genesis", "genesis", false},
		{"validator", "validator", false},
		{"sentry", "sentry", false},
		{"rpc", "rpc", false},
		{"join", "", true},
		{"", "", true},
		{"unknown", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeRole(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeRole(%q): expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeRole(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeRole(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestComposeProfile(t *testing.T) {
	cases := map[string]string{
		"genesis":   "validator",
		"validator": "validator",
		"sentry":    "sentry",
		"rpc":       "rpc",
	}
	for in, want := range cases {
		if got := composeProfile(in); got != want {
			t.Errorf("composeProfile(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyFlagOverridesToConfig(t *testing.T) {
	cfg := &yamlconfig.YamlNodeConfig{
		Role:    "validator",
		Node:    yamlconfig.NodeInfo{Moniker: "original", Environment: "production"},
		Chain:   yamlconfig.ChainInfo{ChainID: "umesh-1", Denom: "uume", MinGasPrice: "0.0025"},
		Network: &yamlconfig.NetworkInfo{},
		Join:    &yamlconfig.JoinInfo{},
	}
	initCmd := newInitCmd()
	if err := initCmd.Flags().Set("moniker", "override"); err != nil {
		t.Fatal(err)
	}
	if err := initCmd.Flags().Set("external-address", "1.2.3.4:26656"); err != nil {
		t.Fatal(err)
	}

	applyFlagOverridesToConfig(initCmd, cfg)

	if cfg.Node.Moniker != "override" {
		t.Errorf("Moniker = %q, want override", cfg.Node.Moniker)
	}
	if cfg.Chain.ChainID != "umesh-1" {
		t.Errorf("ChainID should not change = %q", cfg.Chain.ChainID)
	}
	if cfg.Network == nil || cfg.Network.ExternalAddress != "1.2.3.4:26656" {
		t.Errorf("ExternalAddress = %v", cfg.Network)
	}
	if cfg.Join == nil {
		t.Error("Join should be non-nil after applyFlagOverridesToConfig")
	}
}

func TestPrintInitSummary(t *testing.T) {
	cfg := &yamlconfig.YamlNodeConfig{
		Node:  yamlconfig.NodeInfo{Moniker: "val-1"},
		Chain: yamlconfig.ChainInfo{ChainID: "umesh-1"},
	}
	stdout, _ := captureOutput(func() {
		printInitSummary("validator", "./data-validator", cfg)
	})
	for _, want := range []string{
		"role=validator", "./data-validator", "val-1", "umesh-1",
		"--profile validator up -d",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("summary output missing %q:\n%s", want, stdout)
		}
	}
}

func TestInitCmd_PositionalArgsValidation(t *testing.T) {
	cmd := newInitCmd()
	// No args -> error
	if err := cmd.Args(cmd, nil); err == nil {
		t.Error("expected error for missing role arg")
	}
	// Too many args -> error
	if err := cmd.Args(cmd, []string{"a", "b"}); err == nil {
		t.Error("expected error for too many args")
	}
	// Exactly 1 arg -> OK
	if err := cmd.Args(cmd, []string{"validator"}); err != nil {
		t.Errorf("unexpected error for valid role arg: %v", err)
	}
	if err := cmd.Args(cmd, []string{"sentry"}); err != nil {
		t.Errorf("unexpected error for valid role arg: %v", err)
	}
	if err := cmd.Args(cmd, []string{"rpc"}); err != nil {
		t.Errorf("unexpected error for valid role arg: %v", err)
	}
	if err := cmd.Args(cmd, []string{"genesis"}); err != nil {
		t.Errorf("unexpected error for valid role arg: %v", err)
	}
}
