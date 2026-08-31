package nodeinit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OtelEnabled reports whether the node has an active telemetry config
// (config/otel.yaml exists and is non-empty). An empty file (the default
// created by `umeshd init`) makes the SDK use the noop provider, so nothing
// is exported.
func OtelEnabled() bool {
	b, err := os.ReadFile(filepath.Join(ConfigDir(), "otel.yaml"))
	if err != nil {
		return false
	}
	return len(bytes.TrimSpace(b)) > 0
}

// WriteOtelConfig writes the OpenTelemetry SDK config (otel.yaml) into the
// node config dir, enabling OTLP export for the running node.
//
// Cosmos SDK nodes configure OpenTelemetry through this file
// (~/.umeshd/config/otel.yaml), read at `start`; the standard
// OTEL_EXPORTER_OTLP_* environment variables are ignored by the SDK (the
// opentelemetry-configuration spec states SDKs ignore env vars when a config
// file is present). The file is written during the init command on the host, before
// the container starts.
//
// Telemetry is opt-in: when endpoint is empty the file is left untouched.
// An empty otel.yaml (the default produced by `umeshd init`) makes the SDK
// fall back to the noop provider, so nothing is exported.
//
// WriteOtelConfig is invoked by the init command only when the init flow actually
// runs (fresh node or --force). An idempotent re-run on an already-initialized
// node does not touch the file.
func WriteOtelConfig(serviceName, endpoint, environment string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}
	if serviceName == "" {
		serviceName = "umesh-node"
	}
	if environment == "" {
		environment = "dev"
	}
	cfg := fmt.Sprintf(`file_format: "1.0.0"
resource:
  attributes:
    - name: service.name
      value: %s
    - name: deployment.environment
      value: %s
tracer_provider:
  processors:
    - batch:
        exporter:
          otlp_grpc:
            endpoint: %s
meter_provider:
  readers:
    - periodic:
        exporter:
          otlp_grpc:
            endpoint: %s
logger_provider:
  processors:
    - batch:
        exporter:
          otlp_grpc:
            endpoint: %s
extensions:
  instruments:
    host: {}
    runtime: {}
    diskio: {}
  propagators:
    - tracecontext
`, serviceName, environment, endpoint, endpoint, endpoint)
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(ConfigDir(), "otel.yaml"), []byte(cfg), 0o600); err != nil {
		return fmt.Errorf("write otel.yaml: %w", err)
	}
	return nil
}