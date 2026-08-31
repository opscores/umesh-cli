package cmd

import "testing"

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		level  string
		module string
		want   bool
	}{
		{
			name:   "key-value level matches",
			line:   `time="..." level=info msg="hello"`,
			level:  "info",
			module: "",
			want:   true,
		},
		{
			name:   "key-value level mismatch",
			line:   `time="..." level=info msg="hello"`,
			level:  "error",
			module: "",
			want:   false,
		},
		{
			name:   "json level matches",
			line:   `{"level":"info","module":"state","message":"committed state"}`,
			level:  "info",
			module: "",
			want:   true,
		},
		{
			name:   "json level mismatch",
			line:   `{"level":"info","module":"state","message":"committed state"}`,
			level:  "error",
			module: "",
			want:   false,
		},
		{
			name:   "json module matches",
			line:   `{"level":"info","module":"state","message":"committed state"}`,
			level:  "",
			module: "state",
			want:   true,
		},
		{
			name:   "json module mismatch",
			line:   `{"level":"info","module":"server","module":"consensus","message":"proposal"}`,
			level:  "",
			module: "state",
			want:   false,
		},
		{
			name:   "json consensus module matches despite duplicated module keys",
			line:   `{"level":"info","module":"server","module":"consensus","message":"proposal"}`,
			level:  "",
			module: "consensus",
			want:   true,
		},
		{
			name:   "empty filters match everything",
			line:   `{"level":"info","message":"x"}`,
			level:  "",
			module: "",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilter(tt.line, tt.level, tt.module)
			if got != tt.want {
				t.Errorf("matchesFilter(%q, %q, %q) = %v, want %v", tt.line, tt.level, tt.module, got, tt.want)
			}
		})
	}
}
