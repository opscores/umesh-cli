// Package checks implements host-side diagnostics ported from the
// check-*.sh scripts: architecture, NTP sync, .gitignore hygiene, and node
// readiness. Each check reports a result and whether it failed fatally.
package checks

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Result is the outcome of a single check.
type Result struct {
	Name    string
	OK      bool
	Message string
}

// ErrFatal marks a check that must abort the run.
type ErrFatal struct{ Msg string }

// Error implements error.
func (e *ErrFatal) Error() string { return e.Msg }

// Arch verifies the host is x86_64 with the CPU features required by WasmVM v3
// AOT, has sufficient resources, and that the docker image matches.
//
// minRAMMB and minDiskGB are configurable thresholds. A value <= 0 disables
// the respective check, allowing relaxed requirements for dev/test hosts.
// Critical failures (host arch, image arch, missing CPU features, low RAM)
// are returned as a fatal error; disk/cores shortfalls are reported as warnings.
func Arch(image, projectDir string, minRAMMB, minDiskGB int) ([]Result, error) {
	res := []Result{}
	if runtime.GOARCH != "amd64" {
		return res, &ErrFatal{Msg: fmt.Sprintf("host architecture is %s (expected: amd64)", runtime.GOARCH)}
	}
	res = append(res, Result{Name: "host-arch", OK: true, Message: "host is amd64"})

	if missing := missingCPUFlags(); len(missing) > 0 {
		return res, &ErrFatal{Msg: fmt.Sprintf(
			"missing CPU features required by WasmVM v3 AOT: %s (SSE4.2, AVX, POPCNT, CX16)",
			strings.Join(missing, ", "))}
	}
	res = append(res, Result{Name: "cpu-flags", OK: true, Message: "SSE4.2, AVX, POPCNT, CX16 present"})

	if minRAMMB > 0 {
		ram := totalRAMMB()
		if ram < minRAMMB {
			return res, &ErrFatal{Msg: fmt.Sprintf("insufficient RAM: %dMB (minimum %dMB)", ram, minRAMMB)}
		}
		res = append(res, Result{Name: "ram", OK: true, Message: fmt.Sprintf("RAM: %dMB (minimum %dMB)", ram, minRAMMB)})
	}

	if minDiskGB > 0 {
		disk := freeDiskGB(projectDir)
		if disk < 0 {
			res = append(res, Result{Name: "disk", OK: false, Message: "cannot determine disk space"})
		} else if disk < minDiskGB {
			res = append(res, Result{Name: "disk", OK: false, Message: fmt.Sprintf("low disk space: %dGB available (minimum %dGB)", disk, minDiskGB)})
		} else {
			res = append(res, Result{Name: "disk", OK: true, Message: fmt.Sprintf("disk space: %dGB available", disk)})
		}
	}

	cores, minCores := runtime.NumCPU(), 2
	if cores < minCores {
		res = append(res, Result{Name: "cores", OK: false, Message: fmt.Sprintf("low CPU cores: %d (recommended %d+)", cores, minCores)})
	} else {
		res = append(res, Result{Name: "cores", OK: true, Message: fmt.Sprintf("CPU cores: %d", cores)})
	}

	arch, err := imageArch(image)
	if err != nil {
		return res, &ErrFatal{Msg: fmt.Sprintf("cannot determine image architecture for %s: %v", image, err)}
	}
	if arch != "amd64" {
		return res, &ErrFatal{Msg: fmt.Sprintf("image architecture is %s (expected amd64)", arch)}
	}
	res = append(res, Result{Name: "image-arch", OK: true, Message: "image is amd64"})

	return res, nil
}

// wasmvmRequiredFlags are the CPU features required by WasmVM v3 AOT
// (SSE4.2, AVX, POPCNT, CX16) as advertised in /proc/cpuinfo.
var wasmvmRequiredFlags = []string{"sse4_2", "avx", "popcnt", "cx16"}

// cpuinfoFlags returns the set of CPU flags advertised by the first processor
// in /proc/cpuinfo. It returns an empty set when the file is unreadable.
func cpuinfoFlags() map[string]bool {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	flags := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "flags") {
			continue
		}
		_, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		for _, f := range strings.Fields(after) {
			flags[f] = true
		}
		return flags
	}
	return flags
}

// missingCPUFlags returns the required WasmVM CPU features absent from the host.
func missingCPUFlags() []string {
	flags := cpuinfoFlags()
	var missing []string
	for _, f := range wasmvmRequiredFlags {
		if !flags[f] {
			missing = append(missing, f)
		}
	}
	return missing
}

// totalRAMMB returns total physical memory in MB from /proc/meminfo, or 0 if
// unreadable.
func totalRAMMB() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "MemTotal") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			var kb int
			if _, err := fmt.Sscanf(fields[1], "%d", &kb); err == nil {
				return kb / 1024
			}
		}
		return 0
	}
	return 0
}

// freeDiskGB returns the free disk space of the filesystem containing dir, in
// whole GB, using the system df utility. It returns -1 if undeterminable.
func freeDiskGB(dir string) int {
	out, err := exec.Command("df", "-BG", dir).CombinedOutput()
	if err != nil {
		return -1
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return -1
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return -1
	}
	var gb int
	if _, err := fmt.Sscanf(fields[3], "%d", &gb); err != nil {
		return -1
	}
	return gb
}

func imageArch(image string) (string, error) {
	out, err := exec.Command("docker", "inspect", "--format", "{{.Architecture}}", image).CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveImage returns the Docker image reference to use for inspection/run
// checks. If the requested image is present locally it is used verbatim;
// otherwise (e.g. the default "umesh-node:latest" build artifact is absent in
// a dev setup) it falls back to the image the running node container was
// launched with, discovered via `docker inspect --format '{{.Config.Image}}'
// <container>`. This lets `ops doctor` work out-of-the-box against a node that
// runs a published image (e.g. ghcr.io/opscores/umesh-node:latest) without
// requiring --image or a local umesh-node:latest build.
func ResolveImage(image, container string) string {
	if _, err := exec.Command("docker", "image", "inspect", image).CombinedOutput(); err == nil {
		return image
	}
	if container == "" {
		return image
	}
	if out, err := exec.Command("docker", "inspect",
		"--format", "{{.Config.Image}}", container).CombinedOutput(); err == nil {
		if img := strings.TrimSpace(string(out)); img != "" {
			return img
		}
	}
	return image
}

// NTPSync verifies clock synchronization via chronyc, with timedatectl as a
// fallback. A missing tool is reported as a warning, not a failure.
func NTPSync() ([]Result, error) {
	res := []Result{}
	if offset, ok := chronyOffset(); ok {
		res = append(res, Result{Name: "ntp-offset", OK: true, Message: fmt.Sprintf("offset %dms (chronyc)", offset)})
		return res, nil
	}
	if out, err := exec.Command("timedatectl", "show").CombinedOutput(); err == nil {
		if strings.Contains(string(out), "NTPSynchronized=yes") {
			res = append(res, Result{Name: "ntp", OK: true, Message: "NTP synchronized (timedatectl)"})
			return res, nil
		}
		return res, &ErrFatal{Msg: "NTP not synchronized; run: sudo timedatectl set-ntp true"}
	}
	res = append(res, Result{Name: "ntp", OK: false, Message: "neither chronyc nor timedatectl available"})
	return res, nil
}

// chronyOffset returns the absolute clock offset in milliseconds via chronyc.
func chronyOffset() (int, bool) {
	out, err := exec.Command("chronyc", "tracking").CombinedOutput()
	if err != nil {
		return 0, false
	}
	re := regexp.MustCompile(`Last offset\s*:\s*([-+]?[0-9.]+)`)
	m := re.FindStringSubmatch(string(out))
	if m == nil {
		return 0, false
	}
	var sec float64
	if _, err := fmt.Sscanf(m[1], "%f", &sec); err != nil {
		return 0, false
	}
	ms := int(sec * 1000)
	if ms < 0 {
		ms = -ms
	}
	return ms, true
}

// Gitignore verifies that private keys and env files are excluded from git.
// It checks both .gitignore entries and whether secrets are tracked.
func Gitignore(dir string) ([]Result, error) {
	res := []Result{}
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return res, &ErrFatal{Msg: ".gitignore not found"}
	}
	content := string(gi)
	for _, pattern := range []string{
		"priv_validator_key.json",
		`^\.env$`,
		"^backups/",
	} {
		re := regexp.MustCompile("(?m)" + pattern)
		if re.MatchString(content) {
			res = append(res, Result{Name: "gitignore:" + pattern, OK: true, Message: "present"})
		} else {
			res = append(res, Result{Name: "gitignore:" + pattern, OK: false, Message: "missing from .gitignore"})
		}
	}

	// Secrets already tracked by git are a hard failure regardless of ignore rules.
	if tracked, err := gitLsFiles(dir, "priv_validator_key.json"); err == nil && len(tracked) > 0 {
		return res, &ErrFatal{Msg: fmt.Sprintf(
			"CRITICAL: priv_validator_key.json is tracked by git; remove with: git rm --cached %s",
			strings.Join(tracked, " "))}
	}
	return res, nil
}

// gitLsFiles lists files matching pattern currently tracked by git inside dir.
func gitLsFiles(dir, pattern string) ([]string, error) {
	out, err := exec.Command("git", "-C", dir, "ls-files", "--", pattern).CombinedOutput()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// WasmVMSmoke runs the image in a throwaway read-only container and queries the
// WasmVM runtime version. A failure is reported as a non-fatal result, matching
// the former check-architecture.sh behavior (smoke test is inconclusive).
func WasmVMSmoke(image string) ([]Result, error) {
	out, err := exec.Command("docker", "run", "--rm",
		"--read-only",
		"--tmpfs", "/tmp:rw,nosuid,nodev",
		"--tmpfs", "/home/umesh/.umeshnode:rw,nosuid,nodev,size=64m",
		"--entrypoint", "umeshnode",
		image,
		"query", "wasm", "libwasmvm-version", "--home", "/home/umesh/.umeshnode").CombinedOutput()
	if err != nil {
		return []Result{{Name: "wasmvm-smoke", OK: false,
			Message: fmt.Sprintf("WasmVM runtime test inconclusive: %v", strings.TrimSpace(string(out)))}}, nil
	}
	return []Result{{Name: "wasmvm-smoke", OK: true,
		Message: "WasmVM v3 runtime operational (read-only mode verified)"}}, nil
}

// ContainerHealth reports the docker healthcheck status of the given container.
func ContainerHealth(container string) ([]Result, error) {
	out, err := exec.Command("docker", "inspect", "--format", "{{.State.Health.Status}}", container).CombinedOutput()
	if err != nil {
		return []Result{{Name: "container-health", OK: false,
			Message: fmt.Sprintf("cannot inspect container %s: %v", container, err)}}, nil
	}
	status := strings.TrimSpace(string(out))
	switch status {
	case "healthy":
		return []Result{{Name: "container-health", OK: true, Message: "container healthcheck: healthy"}}, nil
	case "starting":
		return []Result{{Name: "container-health", OK: false, Message: "container healthcheck: starting"}}, nil
	case "none":
		return []Result{{Name: "container-health", OK: false, Message: "container has no healthcheck configured"}}, nil
	default:
		return []Result{{Name: "container-health", OK: false, Message: "container healthcheck: " + status}}, nil
	}
}

// ContainerWasmVM checks the WasmVM runtime inside a running container via
// docker exec. A failure is reported as a warning.
func ContainerWasmVM(container string) ([]Result, error) {
	out, err := exec.Command("docker", "exec", container,
		"umeshnode", "query", "wasm", "libwasmvm-version", "--home", "/home/umesh/.umeshnode").CombinedOutput()
	if err != nil {
		return []Result{{Name: "wasmvm-runtime", OK: false,
			Message: fmt.Sprintf("WasmVM runtime check inconclusive: %v", strings.TrimSpace(string(out)))}}, nil
	}
	return []Result{{Name: "wasmvm-runtime", OK: true,
		Message: "WasmVM runtime is operational"}}, nil
}

// P2PExternalAddress checks p2p.external_address in config.toml.
// Inside a Docker container CometBFT advertises the bridge IP (172.x) when
// external_address is empty — warns, not fatal (per plan decision warning).
func P2PExternalAddress(configDir string) ([]Result, error) {
	cfgPath := filepath.Join(configDir, "config.toml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return []Result{{Name: "p2p-external-address", OK: false, Message: fmt.Sprintf("cannot read %s: %v (hint: run `umeshctl init <role> --config node-config.yaml` first)", cfgPath, err)}}, nil
	}
	content := string(data)
	// naive parse: look for external_address = "..."
	re := regexp.MustCompile(`(?m)^\s*external_address\s*=\s*"([^"]*)"`)
	m := re.FindStringSubmatch(content)
	addr := ""
	if m != nil {
		addr = strings.TrimSpace(m[1])
	}
	switch {
	case addr == "":
		return []Result{{Name: "p2p-external-address", OK: false, Message: "EMPTY — CometBFT will advertise Docker bridge IP 172.x; peers cannot dial. Set network.externalAddress to <public IP>:26656"}}, nil
	case strings.HasPrefix(addr, "tcp://"):
		return []Result{{Name: "p2p-external-address", OK: false, Message: fmt.Sprintf("invalid %q: CometBFT expects host:port without tcp:// (hint: set 203.0.113.10:26656)", addr)}}, nil
	case strings.HasPrefix(addr, "127.") || strings.HasPrefix(addr, "localhost"):
		return []Result{{Name: "p2p-external-address", OK: false, Message: fmt.Sprintf("loopback %q: not reachable from other hosts (hint: use public IP)", addr)}}, nil
	case strings.HasPrefix(addr, "172."):
		return []Result{{Name: "p2p-external-address", OK: false, Message: fmt.Sprintf("bridge %q: likely Docker bridge IP (hint: set public IP, ensure infrastructure allows 26656/tcp)", addr)}}, nil
	default:
		return []Result{{Name: "p2p-external-address", OK: true, Message: fmt.Sprintf("external_address=%q (OK)", addr)}}, nil
	}
}

// JoinReachable checks that at least one join source (genesis URL or RPC) is reachable.
// It is a warning-only check; network isolation or VPN may cause false positives.
// Uses Go net/http directly — no host curl/jq dependency.
func JoinReachable(urls []string) ([]Result, error) {
	if len(urls) == 0 {
		return []Result{{Name: "join-reachable", OK: false, Message: "no join URLs configured (hint: set join.genesisUrl or join.sentryRpc in YAML or --genesis-url/--sentry-rpc flags)"}}, nil
	}
	client := &http.Client{Timeout: 5 * time.Second}
	var res []Result
	for _, u := range urls {
		if u == "" {
			continue
		}
		testURL := u
		if !strings.HasSuffix(testURL, "/genesis") && !strings.HasSuffix(testURL, ".json") {
			testURL = strings.TrimSuffix(testURL, "/") + "/genesis"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
		if err != nil {
			cancel()
			res = append(res, Result{Name: "join:" + u, OK: false, Message: fmt.Sprintf("unreachable %q: %v (hint: verify URL is http(s) with host)", u, err)})
			continue
		}
		resp, err := client.Do(req)
		cancel()
		if err != nil {
			res = append(res, Result{Name: "join:" + u, OK: false, Message: fmt.Sprintf("unreachable %q: %v (hint: try umeshctl genesis fetch --url %q --dry-run)", u, err, u)})
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			res = append(res, Result{Name: "join:" + u, OK: false, Message: fmt.Sprintf("unreachable %q: http %s (%d bytes) hint: umeshctl genesis fetch --url %q --dry-run", u, resp.Status, len(body), u)})
			continue
		}
		res = append(res, Result{Name: "join:" + u, OK: true, Message: fmt.Sprintf("reachable %q (%d bytes)", u, len(body))})
	}
	return res, nil
}
