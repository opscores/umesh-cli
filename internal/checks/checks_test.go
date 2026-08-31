package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingCPUFlags(t *testing.T) {
	got := missingCPUFlags()
	for _, want := range []string{"sse4_2", "avx", "popcnt", "cx16"} {
		if strings.Contains(strings.Join(got, ","), want) {
			t.Fatalf("host should have %s; missingCPUFlags returned %v", want, got)
		}
	}
}

func TestTotalRAMMB(t *testing.T) {
	ram := totalRAMMB()
	if ram <= 0 {
		t.Fatalf("totalRAMMB() = %d, want > 0", ram)
	}
}

func TestFreeDiskGB(t *testing.T) {
	dir, err := os.MkdirTemp("", "umeshctl-checks-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	gb := freeDiskGB(dir)
	if gb < 0 {
		t.Fatalf("freeDiskGB(%s) = %d, want >= 0", dir, gb)
	}
}

func TestGitignore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("priv_validator_key.json\n.env\nbackups/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Gitignore(dir)
	if err != nil {
		t.Fatalf("Gitignore() error: %v", err)
	}
	for _, r := range res {
		if !r.OK {
			t.Errorf("result %q not OK: %s", r.Name, r.Message)
		}
	}
}

func TestGitignoreMissingEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("# empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Gitignore(dir)
	if err != nil {
		t.Fatalf("Gitignore() error: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("got %d results, want 3", len(res))
	}
	for _, r := range res {
		if r.OK {
			t.Errorf("result %q should not be OK", r.Name)
		}
	}
}

func TestGitignoreMissingFile(t *testing.T) {
	if _, err := Gitignore(t.TempDir()); err == nil {
		t.Fatal("Gitignore() should fail when .gitignore is absent")
	}
}

func TestContainerHealthUnhealthy(t *testing.T) {
	res, err := ContainerHealth("definitely-not-a-real-container")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1", len(res))
	}
	if res[0].OK {
		t.Errorf("result %q should not be OK for a nonexistent container", res[0].Name)
	}
}

func TestArchSkipsResourcesWhenThresholdZero(t *testing.T) {
	dir := t.TempDir()
	// Use a nonexistent image so the image-arch check fails; we only assert
	// that the host-side resource checks are skipped when thresholds are 0.
	res, err := Arch("nonexistent-image:latest", dir, 0, 0)
	if err == nil {
		t.Fatal("expected a fatal error from the image-arch check")
	}
	for _, r := range res {
		if r.Name == "ram" || r.Name == "disk" {
			t.Errorf("check %q should be skipped when threshold is 0", r.Name)
		}
	}
}

func TestArchDiskBelowThreshold(t *testing.T) {
	dir, err := os.MkdirTemp("", "umeshctl-arch-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// An astronomically high disk minimum forces the warning path.
	res, _ := Arch("nonexistent-image:latest", dir, 0, 999999)
	found := false
	for _, r := range res {
		if r.Name == "disk" {
			found = true
			if r.OK {
				t.Errorf("disk should not be OK with a 999999GB minimum")
			}
		}
	}
	if !found {
		t.Fatal("expected a disk result when minDiskGB > 0")
	}
}

func TestArchRAMAboveThreshold(t *testing.T) {
	dir := t.TempDir()
	// A tiny RAM minimum should pass on any real host.
	res, err := Arch("nonexistent-image:latest", dir, 1, 0)
	if err == nil {
		t.Fatal("expected a fatal error from the image-arch check")
	}
	found := false
	for _, r := range res {
		if r.Name == "ram" {
			found = true
			if !r.OK {
				t.Errorf("ram should be OK with a 1MB minimum")
			}
		}
	}
	if !found {
		t.Fatal("expected a ram result when minRAMMB > 0")
	}
}
