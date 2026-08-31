package uio

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// OutputFormat enumerates supported structured-output formats.
type OutputFormat string

const (
	FormatTable OutputFormat = "table"
	FormatJSON  OutputFormat = "json"
	FormatYAML  OutputFormat = "yaml"
)

// ParseOutputFormat normalizes an --output value. Empty means table.
func ParseOutputFormat(s string) (OutputFormat, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "table", "text":
		return FormatTable, nil
	case "json":
		return FormatJSON, nil
	case "yaml", "yml":
		return FormatYAML, nil
	default:
		return "", fmt.Errorf("invalid output format %q: must be table, json, or yaml", s)
	}
}

// OutputFormatChoices returns the list of supported output format strings for help text.
func OutputFormatChoices() []string {
	return []string{"table", "text", "json", "yaml", "yml"}
}

// Emit writes v to stdout in the requested format.
// table delegates to tableFn, which is responsible for printing a human
// readable representation. json/yaml serialize v.
//
// Contract (kept consistent across commands):
//   - `v` is the single source of truth for machine-readable output: it MUST
//     use stable snake_case keys (e.g. "connected_peers", "persistent_peers"),
//     since these are the names piped to jq/pipelines.
//   - tableFn is only invoked for FormatTable, so any Print calls inside it
//     NEVER leak into json/yaml stdout (see cmd/peers.go, cmd/status.go).
//   - Human table labels MAY differ phrasing from the JSON key (e.g.
//     "Connected Peers" vs "connected_peers"), but each label MUST describe the
//     field printed by the matching JSON key, so table and json stay
//     semantically aligned and greppable.
//   - Destructive/operational messages (Confirm, LogStep/LogSuccess/LogInfo)
//     go to stderr where possible, or are guarded behind `format == FormatTable`
//     / placed inside tableFn, so they never pollute --output json|yaml.
func Emit(format OutputFormat, v any, tableFn func()) error {
	switch format {
	case FormatJSON:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		_, _ = fmt.Fprintln(os.Stdout, string(b))
		return nil
	case FormatYAML:
		b, err := marshalYAML(v)
		if err != nil {
			return fmt.Errorf("encode yaml: %w", err)
		}
		_, _ = fmt.Fprintln(os.Stdout, strings.TrimRight(string(b), "\n"))
		return nil
	default:
		tableFn()
		return nil
	}
}

func marshalYAML(v any) ([]byte, error) {
	return yaml.Marshal(v)
}