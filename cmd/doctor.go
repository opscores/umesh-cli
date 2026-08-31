package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/checks"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/role"
	"github.com/opscores/umesh-cli/internal/rpcclient"
	"github.com/opscores/umesh-cli/internal/uio"
)

// doctorCheck is a single named diagnostic.
type doctorCheck struct {
	Name   string `json:"name" yaml:"name"`
	Status string `json:"status" yaml:"status"` // ok / warn / fail
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`
	Fix    string `json:"fix,omitempty" yaml:"fix,omitempty"`
}

// doctorReport is the structured output of ops doctor.
type doctorReport struct {
	Role    string        `json:"role,omitempty" yaml:"role,omitempty"`
	Checks  []doctorCheck `json:"checks" yaml:"checks"`
	Summary doctorSummary `json:"summary" yaml:"summary"`
}

type doctorSummary struct {
	OK   int `json:"ok" yaml:"ok"`
	Warn int `json:"warn" yaml:"warn"`
	Fail int `json:"fail" yaml:"fail"`
}

func newDoctorCmd() *cobra.Command {
	var only string
	var minRAMMB, minDiskGB int
	var roleOverride string
	var all bool
	var output string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostic checks",
		Long: `Run host + node diagnostic checks.

Default runs the core checks (arch, ntp, gitignore, readiness, p2p, join).
Use --check to run a single component, --all to run every check including
container and wasmvm, and --output json/yaml for machine-readable output.

Resource thresholds for the "arch" check default to production minima
(4096MB RAM, 50GB disk). Override them for dev/test hosts:
  doctor --check arch --min-ram-mb 512 --min-disk-gb 5`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}

			var report doctorReport
			if roleOverride != "" {
				if r, err := role.Resolve(global.DataDir, roleOverride); err == nil {
					report.Role = r
				}
			} else if r, err := role.Resolve(global.DataDir, ""); err == nil {
				report.Role = r
			}

			// Define the checks to run and a one-line "how to fix" for each.
			type spec struct {
				name string
				run  func() ([]checks.Result, error)
				fix  string
			}
			image := checks.ResolveImage(global.Image, global.Container)
			specs := []spec{
				{"arch", func() ([]checks.Result, error) {
					return checks.Arch(image, projectRoot(), minRAMMB, minDiskGB)
				}, "Check CPU/RAM/disk requirements; see README §2"},
				{"ntp", func() ([]checks.Result, error) {
					return checks.NTPSync()
				}, "Install/enable chrony or ntpd and verify time sync"},
				{"gitignore", func() ([]checks.Result, error) {
					return checks.Gitignore(projectRoot())
				}, "Add the reported paths to .gitignore"},
				{"readiness", func() ([]checks.Result, error) {
					return readinessResults(roleOverride)
				}, "Ensure the node container is running and RPC is reachable"},
				{"wasmvm", func() ([]checks.Result, error) {
					return checks.WasmVMSmoke(image)
				}, "Ensure the umesh-node image bundles the matching WasmVM"},
				{"container-health", func() ([]checks.Result, error) {
					return checks.ContainerHealth(global.Container)
				}, "Start the container: docker compose --profile <role> up -d"},
				{"peers", func() ([]checks.Result, error) {
					return nodePeersResults()
				}, "Check p2p port reachability and seed/persistent-peers config"},
				{"p2p", func() ([]checks.Result, error) {
					return checks.P2PExternalAddress(nodeinit.ConfigDir())
				}, "Set network.externalAddress to the public IP:26656"},
				{"join", func() ([]checks.Result, error) {
					return checks.JoinReachable([]string{global.RPCURL})
				}, "Verify the genesis source is reachable"},
			}

			// Selection: --check <name> | --all | default core set.
			core := map[string]bool{"arch": true, "ntp": true, "gitignore": true, "readiness": true, "p2p": true, "join": true}
			selected := map[string]bool{}
			switch {
			case only != "":
				found := false
				for _, s := range specs {
					if s.name == only {
						found = true
						break
					}
				}
				if !found {
					return &checks.ErrFatal{Msg: "unknown check: " + only}
				}
				selected[only] = true
			case all:
				for _, s := range specs {
					selected[s.name] = true
				}
			default:
				for name := range core {
					selected[name] = true
				}
			}

			for _, s := range specs {
				if !selected[s.name] {
					continue
				}
				res, err := s.run()
				appendDoctorResults(&report, s.name, res, err, s.fix)
			}

			if format != uio.FormatTable {
				if err := uio.Emit(format, report, func() {}); err != nil {
					return err
				}
				if report.Summary.Fail > 0 {
					return &checks.ErrFatal{Msg: fmt.Sprintf("%d check(s) failed", report.Summary.Fail)}
				}
				return nil
			}

			// Human-readable summary.
			uio.LogStep("Doctor report (role: %s)", report.Role)
			for _, c := range report.Checks {
				switch c.Status {
				case "ok":
					uio.LogSuccess("%-20s %s", c.Name, c.Detail)
				case "warn":
					uio.LogWarning("%-20s %s", c.Name, c.Detail)
					if c.Fix != "" {
						uio.LogInfo("  fix: %s", c.Fix)
					}
				default:
					uio.LogError("%-20s %s", c.Name, c.Detail)
					if c.Fix != "" {
						uio.LogInfo("  fix: %s", c.Fix)
					}
				}
			}
			uio.LogInfo("Summary: %d ok, %d warn, %d fail", report.Summary.OK, report.Summary.Warn, report.Summary.Fail)

			if report.Summary.Fail > 0 {
				return &checks.ErrFatal{Msg: fmt.Sprintf("%d check(s) failed", report.Summary.Fail)}
			}
			uio.LogSuccess("All selected checks passed (p2p/join warnings are non-fatal)")
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "check", "", "Run only one check (arch/ntp/gitignore/readiness/wasmvm/container-health/peers/p2p/join)")
	cmd.Flags().BoolVar(&all, "all", false, "Run every check including container and wasmvm")
	cmd.Flags().IntVar(&minRAMMB, "min-ram-mb", 4096, "Minimum RAM in MB for the arch check (0 disables)")
	cmd.Flags().IntVar(&minDiskGB, "min-disk-gb", 50, "Minimum free disk in GB for the arch check (0 disables)")
	cmd.Flags().StringVar(&roleOverride, "role", "", "Node role override (auto-detected if empty)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("role", completeRoles())
	_ = cmd.RegisterFlagCompletionFunc("check", completeDoctorChecks())
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

// appendDoctorResults folds []checks.Result + error into the report.
func appendDoctorResults(report *doctorReport, name string, res []checks.Result, err error, fix string) {
	// Group all sub-results of a check under a single entry when there are
	// several; otherwise use the single result's fields.
	if len(res) == 0 {
		status := "fail"
		detail := err.Error()
		if err == nil {
			status, detail = "ok", "passed"
		}
		report.Checks = append(report.Checks, doctorCheck{Name: name, Status: status, Detail: detail, Fix: fix})
		report.Summary.Warn += 0
		report.Summary.Fail += boolToInt(err != nil)
		return
	}

	failed := 0
	warned := 0
	var details []string
	for _, r := range res {
		if !r.OK {
			warned++
			details = append(details, fmt.Sprintf("%s: %s", r.Name, r.Message))
		} else {
			details = append(details, fmt.Sprintf("%s: %s", r.Name, r.Message))
		}
	}
	// A fatal error on a check flags the whole check as failed.
	if err != nil {
		var fe *checks.ErrFatal
		if errors.As(err, &fe) {
			failed = 1
		}
	}
	detail := details[0]
	if len(details) > 1 {
		detail = fmt.Sprintf("%s (+%d more)", details[0], len(details)-1)
	}
	status := "ok"
	switch {
	case failed > 0:
		status = "fail"
		// Prefer the fatal error message as the primary detail.
		var fe *checks.ErrFatal
		if errors.As(err, &fe) {
			detail = fe.Msg
		}
	case warned > 0:
		status = "warn"
	}
	report.Checks = append(report.Checks, doctorCheck{Name: name, Status: status, Detail: detail, Fix: fix})
	switch status {
	case "fail":
		report.Summary.Fail++
	case "warn":
		report.Summary.Warn++
	default:
		report.Summary.OK++
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func readinessResults(roleOverride string) ([]checks.Result, error) {
	role, err := role.Resolve(global.DataDir, roleOverride)
	if err != nil {
		return nil, fmt.Errorf("resolve role: %w", err)
	}
	client := rpcclient.New(global.RPCURL)
	st, err := client.Status()
	if err != nil {
		return nil, &checks.ErrFatal{Msg: "cannot connect to node RPC: " + err.Error()}
	}
	results := []checks.Result{
		{Name: "rpc-status", OK: true, Message: fmt.Sprintf("role=%s height=%s catching_up=%v", role, st.SyncInfo.LatestBlockHeight, st.SyncInfo.CatchingUp)},
	}
	if ni, err := client.NetInfo(); err == nil {
		results = append(results, checks.Result{Name: "peers", OK: ni.NPeers > 0, Message: fmt.Sprintf("%d peers connected", ni.NPeers)})
	}
	if role == "sentry" || role == "rpc" {
		if err := client.Health(); err != nil {
			return nil, &checks.ErrFatal{Msg: "API endpoint not accessible: " + err.Error()}
		}
		results = append(results, checks.Result{Name: "api-health", OK: true, Message: "API endpoint accessible"})
	}
	return results, nil
}

// nodePeersResults returns peer connectivity results without printing.
func nodePeersResults() ([]checks.Result, error) {
	client := rpcclient.New(global.RPCURL)
	ni, err := client.NetInfo()
	if err != nil {
		return nil, &checks.ErrFatal{Msg: "cannot reach net_info: " + err.Error()}
	}
	results := []checks.Result{{Name: "peers", OK: ni.NPeers > 0, Message: fmt.Sprintf("%d peers connected", ni.NPeers)}}
	return results, nil
}
