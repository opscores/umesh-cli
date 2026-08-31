package secret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "password")
	if err := os.WriteFile(path, []byte("mysecret123\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := Source{File: path}
	got, err := Resolve(s)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != "mysecret123" {
		t.Errorf("got %q, want %q", got, "mysecret123")
	}
}

func TestResolveFromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	go func() {
		if _, err := w.Write([]byte("stdinsecret\n")); err != nil {
			t.Errorf("write: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	s := Source{Stdin: true}
	got, err := Resolve(s)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != "stdinsecret" {
		t.Errorf("got %q, want %q", got, "stdinsecret")
	}
}

func TestResolveFromExec(t *testing.T) {
	s := Source{Exec: "echo myexecsecret"}
	got, err := Resolve(s)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != "myexecsecret" {
		t.Errorf("got %q, want %q", got, "myexecsecret")
	}
}

func TestResolveZeroReturnsError(t *testing.T) {
	s := Source{}
	_, err := Resolve(s)
	if err == nil {
		t.Error("expected error for zero-value secret source")
	}
}
