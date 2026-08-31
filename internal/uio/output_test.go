package uio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOutputFormat(t *testing.T) {
	cases := []struct {
		in      string
		want    OutputFormat
		wantErr bool
	}{
		{"", FormatTable, false},
		{"table", FormatTable, false},
		{"text", FormatTable, false},
		{"json", FormatJSON, false},
		{"JSON", FormatJSON, false},
		{"yaml", FormatYAML, false},
		{"yml", FormatYAML, false},
		{"xml", "", true},
	}
	for _, c := range cases {
		got, err := ParseOutputFormat(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseOutputFormat(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("ParseOutputFormat(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestEmitJSON(t *testing.T) {
	dir := t.TempDir()
	stdout := filepath.Join(dir, "out")
	old := os.Stdout
	f, err := os.Create(stdout)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = f
	defer func() { os.Stdout = old }()

	err = Emit(FormatJSON, map[string]any{"a": 1, "b": "x"}, func() {})
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(stdout)
	if !strings.Contains(string(got), `"a": 1`) {
		t.Errorf("json output missing key: %s", got)
	}
}

func TestEmitYAML(t *testing.T) {
	dir := t.TempDir()
	stdout := filepath.Join(dir, "out")
	old := os.Stdout
	f, err := os.Create(stdout)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = f
	defer func() { os.Stdout = old }()

	err = Emit(FormatYAML, map[string]any{"a": 1, "b": "x"}, func() {})
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(stdout)
	if !strings.Contains(string(got), "a: 1") {
		t.Errorf("yaml output missing key: %s", got)
	}
}