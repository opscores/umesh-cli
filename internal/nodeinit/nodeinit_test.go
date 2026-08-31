package nodeinit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAbortIfInitializedClean(t *testing.T) {
	dir := t.TempDir()
	SetHome(filepath.Join(dir, "data"))

	if err := AbortIfInitialized(); err != nil {
		t.Fatalf("AbortIfInitialized() on clean home returned err: %v", err)
	}
}

func TestAbortIfInitializedExisting(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	// Simulate an existing initialization.
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GenesisFile(), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := AbortIfInitialized()
	if !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("AbortIfInitialized() = %v, want ErrAlreadyInitialized", err)
	}
}

func TestAbortIfInitializedForce(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	// Simulate an existing initialization.
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GenesisFile(), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// With ForceReinit=true, the guard should pass.
	orig := ForceReinit
	ForceReinit = true
	defer func() { ForceReinit = orig }()

	if err := AbortIfInitialized(); err != nil {
		t.Fatalf("AbortIfInitialized() with ForceReinit = true returned err: %v", err)
	}
}

func TestRunGenesisIdempotent(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	if err := os.MkdirAll(filepath.Join(home, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GenesisFile(), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// RunGenesis hits AbortIfInitialized before any umeshd call, so this
	// works without the umeshd binary present.
	err := RunGenesis(GenesisParams{
		ChainID: "test-1", Moniker: "node", Denom: "stake",
		MinGasPrice: "0.025stake", Environment: "test",
	})
	if !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("RunGenesis() = %v, want ErrAlreadyInitialized", err)
	}
}

func TestRunValidatorIdempotent(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	if err := os.MkdirAll(filepath.Join(home, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GenesisFile(), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RunValidator(ValidatorParams{
		ChainID: "test-1", Moniker: "node", Denom: "stake",
		MinGasPrice: "0.025stake", Environment: "test",
	})
	if !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("RunValidator() = %v, want ErrAlreadyInitialized", err)
	}
}

func TestRunSentryIdempotent(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	if err := os.MkdirAll(filepath.Join(home, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GenesisFile(), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RunSentry(SentryParams{
		ChainID: "test-1", Moniker: "node", Denom: "stake",
		MinGasPrice: "0.025stake", Environment: "test",
	})
	if !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("RunSentry() = %v, want ErrAlreadyInitialized", err)
	}
}

func TestRunRPCIdempotent(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	SetHome(home)

	if err := os.MkdirAll(filepath.Join(home, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GenesisFile(), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RunRPC(RPCParams{
		ChainID: "test-1", Moniker: "node", Denom: "stake",
		MinGasPrice: "0.025stake", Environment: "test",
	})
	if !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("RunRPC() = %v, want ErrAlreadyInitialized", err)
	}
}

func TestCaptureRestoreIdentityKeys(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")

	if err := os.MkdirAll(filepath.Join(home, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	origKey := []byte(`{"address":"A","pub_key":{"type":"t","value":"v"}}`)
	origNode := []byte(`{"priv_key":{"type":"t","value":"n"}}`)
	if err := os.WriteFile(privValidatorKeyFile(home), origKey, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodeKeyFile(home), origNode, 0o600); err != nil {
		t.Fatal(err)
	}

	ik, err := captureIdentityKeys(home)
	if err != nil {
		t.Fatalf("captureIdentityKeys() error: %v", err)
	}

	// Simulate `umeshd init --overwrite` replacing both files.
	if err := os.WriteFile(privValidatorKeyFile(home), []byte(`{"address":"NEW"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodeKeyFile(home), []byte(`{"priv_key":{"type":"t","value":"NEW"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ik.restore(home); err != nil {
		t.Fatalf("restore() error: %v", err)
	}

	gotKey, err := os.ReadFile(privValidatorKeyFile(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotKey) != string(origKey) {
		t.Errorf("priv_validator_key.json = %q, want %q", gotKey, origKey)
	}
	gotNode, err := os.ReadFile(nodeKeyFile(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotNode) != string(origNode) {
		t.Errorf("node_key.json = %q, want %q", gotNode, origNode)
	}
}

func TestCaptureRestoreIdentityKeysMissingFiles(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")

	if err := os.MkdirAll(filepath.Join(home, "config"), 0o750); err != nil {
		t.Fatal(err)
	}

	// No identity files exist: capture must not error and restore must not
	// create anything.
	ik, err := captureIdentityKeys(home)
	if err != nil {
		t.Fatalf("captureIdentityKeys() error: %v", err)
	}
	if err := ik.restore(home); err != nil {
		t.Fatalf("restore() error: %v", err)
	}
	if _, err := os.Stat(privValidatorKeyFile(home)); !os.IsNotExist(err) {
		t.Error("priv_validator_key.json should not have been created")
	}
	if _, err := os.Stat(nodeKeyFile(home)); !os.IsNotExist(err) {
		t.Error("node_key.json should not have been created")
	}
}

func TestResetChainState(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "data")
	dataDir := filepath.Join(home, "data")

	for _, name := range []string{
		"blockstore.db", "state.db", "application.db",
		"evidence.db", "tx_index.db", "cs.wal", "snapshots",
	} {
		if err := os.MkdirAll(filepath.Join(dataDir, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	// priv_validator_state.json must survive the reset.
	if err := os.MkdirAll(filepath.Join(home, "data"), 0o750); err != nil {
		t.Fatal(err)
	}
	state := `{"height":"5","round":0,"step":0}`
	if err := os.WriteFile(filepath.Join(dataDir, "priv_validator_state.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	// A stray non-state file (e.g. backups) must survive too.
	stray := filepath.Join(dataDir, "README.txt")
	if err := os.WriteFile(stray, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := resetChainState(home); err != nil {
		t.Fatalf("resetChainState() error: %v", err)
	}

	for _, name := range []string{
		"blockstore.db", "state.db", "application.db",
		"evidence.db", "tx_index.db", "cs.wal", "snapshots",
	} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); !os.IsNotExist(err) {
			t.Errorf("%s not removed after reset", name)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dataDir, "priv_validator_state.json")); err != nil || string(got) != state {
		t.Errorf("priv_validator_state.json = %q, err=%v, want %q preserved", got, err, state)
	}
	if got, err := os.ReadFile(stray); err != nil || string(got) != "keep me" {
		t.Errorf("non-state file not preserved: %q, err=%v", got, err)
	}
}
