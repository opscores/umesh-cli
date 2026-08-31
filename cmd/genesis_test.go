package cmd

import "testing"

func TestMapAccountType(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"base", "base_account", false},
		{"base_account", "base_account", false},
		{"delayed_vesting", "delayed_vesting", false},
		{"continuous_vesting", "continuous_vesting", false},
		{"module", "module_account", false},
		{"module_account", "module_account", false},
		{"delayedvesting", "", true},
		{"clawback_vesting", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		got, err := mapAccountType(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("mapAccountType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("mapAccountType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractDenom(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"5000uumesh", "uumesh"},
		{"100000000000000stake", "stake"},
		{"5000", ""},
	}

	for _, tt := range tests {
		if got := extractDenom(tt.input); got != tt.want {
			t.Errorf("extractDenom(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
