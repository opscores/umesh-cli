// Package dkrcmd is a thin, dependency-free wrapper around the docker CLI.
//
// It supports two interaction modes:
//   - Running: the target container is up; commands are executed via
//     `docker exec` and logs via `docker logs`.
//   - Offline: the container is stopped; operations that need the image run
//     through `docker run --rm` with the project's data volumes mounted at the
//     node's expected paths.
//
// Using the docker CLI rather than the Docker SDK keeps the binary light and
// avoids pulling heavy dependencies into the image build (where umeshctl is
// also installed).
package dkrcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Docker wraps the docker CLI.
type Docker struct {
	// Container is the target container name (default: umesh-validator).
	Container string
	// Image is used for offline `docker run --rm` operations.
	Image string
	// Home is the node home path used for volume mounts.
	Home string
	// DataDir is the host path mounted at Home (e.g. ./data).
	DataDir string
	// BackupsDir is the host path mounted at Home/backups.
	BackupsDir string
	// Network is the Docker network to attach to (empty = default bridge).
	Network string
	// ExtraMounts are additional host->container bind mounts applied on top of
	// the standard data mounts (e.g. a snapshot restore source directory).
	ExtraMounts []struct{ Host, Container string }
}

// Opt configures a Docker.
type Opt func(*Docker)

// WithContainer sets the target container name.
func WithContainer(name string) Opt { return func(d *Docker) { d.Container = name } }

// WithImage sets the image used for offline operations.
func WithImage(image string) Opt { return func(d *Docker) { d.Image = image } }

// WithHome sets the in-container home path.
func WithHome(home string) Opt { return func(d *Docker) { d.Home = home } }

// WithDataDir sets the host data mount path.
func WithDataDir(dir string) Opt { return func(d *Docker) { d.DataDir = dir } }

// WithBackupsDir sets the host backups mount path.
func WithBackupsDir(dir string) Opt { return func(d *Docker) { d.BackupsDir = dir } }

// WithNetwork sets the Docker network for run/exec operations.
func WithNetwork(net string) Opt { return func(d *Docker) { d.Network = net } }

// WithExtraMount adds an extra host->container bind mount used by RunMount.
func WithExtraMount(host, container string) Opt {
	return func(d *Docker) {
		d.ExtraMounts = append(d.ExtraMounts, struct{ Host, Container string }{host, container})
	}
}

// New builds a Docker with defaults overridden by opts.
func New(opts ...Opt) *Docker {
	d := &Docker{
		Container:  "umesh-validator",
		Image:      "umesh-node:latest",
		Home:       "/home/umesh/.umeshnode",
		DataDir:    "./data",
		BackupsDir: "./backups",
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// run executes a docker command, streaming stderr, and returns stdout.
func (d *Docker) run(stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.Command("docker", args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// EnsureNetwork makes sure a Docker network exists, creating it if missing.
// It runs before any throwaway container attaches to the network, since those
// container runs happen on the host during setup — before docker compose (which
// declares the network as external) has been started.
func (d *Docker) EnsureNetwork(name string) error {
	if _, err := d.run(nil, "network", "inspect", name); err == nil {
		return nil // already exists
	}
	if _, err := d.run(nil, "network", "create", name); err != nil {
		return fmt.Errorf("create docker network %s: %w", name, err)
	}
	return nil
}

// Preflight verifies that the docker CLI is available, the daemon is running,
// and the image used for offline operations exists locally. Offline
// init/keys/genesis flows require all three, and the raw docker errors
// ("exit status 125", "Unable to find image") are cryptic without a hint.
func (d *Docker) Preflight() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found on PATH — install Docker first")
	}
	if _, err := d.run(nil, "version", "--format", "{{.Server.Version}}"); err != nil {
		return fmt.Errorf("docker daemon not reachable — is the Docker service running? (%v)", err)
	}
	if _, err := d.run(nil, "image", "inspect", d.Image); err != nil {
		return fmt.Errorf("image %q not found locally — build it or pull it (docker pull %s)", d.Image, d.Image)
	}
	return nil
}

// IsRunning reports whether the target container is currently running.
// Returns false if the container does not exist or is not running.
func (d *Docker) IsRunning() bool {
	out, err := d.run(nil, "inspect", "-f", "{{.State.Running}}", d.Container)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// Inspect runs `docker inspect --format <format> <target>` against the host
// Docker daemon. Unlike Exec (which runs inside the container), this reads
// container/image metadata such as the healthcheck status or the configured
// image reference. target may be a container name/id or an image name/id.
func (d *Docker) Inspect(format, target string) (string, error) {
	out, err := d.run(nil, "inspect", "--format", format, target)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Exec runs an arbitrary command inside the running container. stdin may be
// nil. It returns the combined output for downstream parsing.
func (d *Docker) Exec(stdin io.Reader, args ...string) ([]byte, error) {
	argv := append([]string{"exec", "-i", d.Container}, args...)
	return d.run(stdin, argv...)
}

// ExecOutput runs a command in the container and returns trimmed stdout.
func (d *Docker) ExecOutput(args ...string) (string, error) {
	out, err := d.Exec(nil, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RunMount executes a command in a throwaway container with the project's data
// volumes mounted at the node home. Used when the node container is stopped.
// stdin may be nil.
func (d *Docker) RunMount(stdin io.Reader, args ...string) ([]byte, error) {
	home := d.Home
	// Run as the umesh uid/gid (1000:1000, pinned in the Dockerfile). mountFlags
	// below pre-creates the host data dirs as the current user, so a bind mount
	// that does not exist yet is never auto-created root:root (which would make
	// it unwritable for the -u 1000:1000 offline processes).
	flags := []string{"run", "--rm", "-u", "1000:1000"}
	if stdin != nil {
		flags = append(flags, "-i")
	}
	if d.Network != "" {
		// The network is needed BEFORE docker compose up: throwaway containers
		// attach to it during setup (init/plan/keys), which compose has not
		// declared/rcreated yet. docker-compose.yml marks it external:true, so
		// compose cannot create it — ensure it exists here.
		if err := d.EnsureNetwork(d.Network); err != nil {
			return nil, err
		}
		flags = append(flags, "--network", d.Network)
	}
	// Ensure the host data directories exist and are owned by the current
	// user before mounting. Docker auto-creates missing bind-mount sources as
	// root:root, which makes them unwritable for the container's -u 1000:1000
	// offline processes (and would break init/keys/genesis flows).
	mounts := []struct {
		host, container string
	}{
		{d.DataDir + "/config", home + "/config"},
		{d.DataDir + "/data", home + "/data"},
		{d.DataDir + "/wasm", home + "/wasm"},
		{d.DataDir + "/keyring", home + "/keyring"},
		{d.BackupsDir, home + "/backups"},
	}
	for _, m := range d.ExtraMounts {
		mounts = append(mounts, struct{ host, container string }{m.Host, m.Container})
	}
	mountFlags, err := d.mountFlags(mounts)
	if err != nil {
		return nil, err
	}
	volumeArgs := append(flags, mountFlags...)
	volumeArgs = append(volumeArgs, d.Image)
	volumeArgs = append(volumeArgs, args...)
	return d.run(stdin, volumeArgs...)
}

// mountFlags builds the "-v host:container" list for the data mounts, first
// ensuring each host directory exists with owner-writable permissions.
func (d *Docker) mountFlags(mounts []struct{ host, container string }) ([]string, error) {
	flags := make([]string, 0, len(mounts)*2)
	for _, m := range mounts {
		if err := os.MkdirAll(m.host, 0o750); err != nil {
			return nil, fmt.Errorf("create data dir %s: %w", m.host, err)
		}
		flags = append(flags, "-v", m.host+":"+m.container)
	}
	return flags, nil
}
