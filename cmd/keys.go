package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/nodeinit"
	"github.com/opscores/umesh-cli/internal/uio"
)

// newKeysAddCmd creates the command for adding a new key (setup phase).
func newKeysAddCmd() *cobra.Command {
	var keyringPasswordFile string
	var keyringPasswordStdin bool
	var keyringPasswordExec string
	var output string

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new key to keyring",
		Long:  "Add a new key to the keyring. You will be prompted for the keyring password.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			pw, err := resolveKeyringPassword(cmd, false)
			if err != nil {
				return err
			}
			var input io.Reader
			if pw != "" {
				input = strings.NewReader(pw + "\n")
			} else {
				uio.LogInfo("Enter keyring password (input is hidden):")
			}
			return execKeys(input, "add", args[0], "--output", string(format))
		},
	}

	f := cmd.Flags()
	f.StringVarP(&keyringPasswordFile, "keyring-password-file", "p", "", "Read keyring password from file (alias: -p)")
	f.BoolVar(&keyringPasswordStdin, "keyring-password-stdin", false, "Read keyring password from stdin")
	f.StringVar(&keyringPasswordExec, "keyring-password-exec", "", "Execute command and read keyring password from stdout")
	f.StringVarP(&output, "output", "o", "json", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())

	return cmd
}

// newKeysRuntimeCmd creates the parent command for viewing/managing keys (runtime phase).
func newKeysRuntimeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "View and manage keys (runtime)",
		Long: `Key management commands for a running node.

  umeshctl node keys list                # list all keys
  umeshctl node keys show <name>         # show key info
  umeshctl node keys export <name>       # export key (armored JSON)
  umeshctl node keys import <name>       # import key
  umeshctl node keys delete <name>       # delete key`,
	}
	cmd.AddCommand(
		newKeysListCmd(),
		newKeysShowCmd(),
		newKeysExportCmd(),
		newKeysImportCmd(),
		newKeysDeleteCmd(),
	)
	return cmd
}

func newKeysListCmd() *cobra.Command {
	var keyringPass, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all keys",
		Long: `List all keys in the keyring.

  umeshctl node keys list            # table (default)
  umeshctl node keys list -o json   # machine-readable
  umeshctl node keys list --keyring-pass <pass>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			// Always ask umeshd for JSON, then render consistently in the
			// requested format (avoids raw umeshd table leaking to users).
			out, err := execKeysRaw(strings.NewReader(keyringPass+"\n"), "list", "--output", "json")
			if err != nil {
				return fmt.Errorf("list keys: %w", err)
			}
			keys, perr := unmarshalKeysJSON(out)
			if perr != nil {
				preview := out
				if len(preview) > 200 {
					preview = preview[:200]
				}
				return fmt.Errorf("parse umeshd keys list output: %w\nfirst bytes: %.200s", perr, preview)
			}
			return uio.Emit(format, keys, func() {
				for _, k := range keys {
					name, _ := k["name"].(string)
					address, _ := k["address"].(string)
					typ, _ := k["type"].(string)
					uio.Print("Name:    %s", name)
					uio.Print("Address: %s", address)
					uio.Print("Type:    %s", typ)
				}
			})
		},
	}
	cmd.Flags().StringVar(&keyringPass, "keyring-pass", "", "Keyring password (inline; for CI prefer --keyring-password-file/-p on setup commands)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	return cmd
}

func newKeysShowCmd() *cobra.Command {
	var keyringPass, output string
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show key info",
		Long: `Show details of a key in the keyring.

  umeshctl node keys show mykey            # json (default)
  umeshctl node keys show mykey -o table   # human table
  umeshctl node keys show mykey -o yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := uio.ParseOutputFormat(output)
			if err != nil {
				return err
			}
			out, err := execKeysRaw(strings.NewReader(keyringPass+"\n"), "show", args[0], "--output", "json")
			if err != nil {
				return fmt.Errorf("show key: %w", err)
			}
			var info map[string]any
			if perr := json.Unmarshal(out, &info); perr != nil {
				preview := out
				if len(preview) > 200 {
					preview = preview[:200]
				}
				return fmt.Errorf("parse umeshd show output: %w\nfirst bytes: %.200s", perr, preview)
			}
			return uio.Emit(format, info, func() {
				name, _ := info["name"].(string)
				address, _ := info["address"].(string)
				pubkey, _ := info["pubkey"].(string)
				typ, _ := info["type"].(string)
				uio.Print("Name:    %s", name)
				uio.Print("Address: %s", address)
				uio.Print("PubKey:  %s", pubkey)
				uio.Print("Type:    %s", typ)
			})
		},
	}
	cmd.Flags().StringVar(&keyringPass, "keyring-pass", "", "Keyring password (inline; for CI prefer --keyring-password-file/-p on setup commands)")
	cmd.Flags().StringVarP(&output, "output", "o", "json", "Output format: table, text, json, yaml, yml")
	_ = cmd.RegisterFlagCompletionFunc("output", completeOutputFormats())
	cmd.ValidArgsFunction = completeKeyNames()
	return cmd
}

func newKeysExportCmd() *cobra.Command {
	var keyringPass string
	cmd := &cobra.Command{
		Use:   "export <name>",
		Short: "Export a key's private key as unarmored hex (unsafe)",
		Long: `Export a key as an unarmored hex private key.

WARNING: This writes the raw private key to stdout. Never export keys on a
production node; use it only on a test/dev validator and shred the output
immediately.

cosmos-sdk 0.54 gates this behind --unsafe and a confirmation; this command
passes --unsafe -y --unarmored-hex along with the keyring password.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// cosmos-sdk 0.54 requires --unsafe (-y to skip the confirm prompt)
			// for unarmored-hex export; armored-JSON export is not supported
			// by umeshnode keys.
			return execKeys(strings.NewReader(keyringPass+"\n"), "export", args[0], "--unsafe", "-y", "--unarmored-hex")
		},
	}
	cmd.Flags().StringVar(&keyringPass, "keyring-pass", "", "Keyring password (inline; for CI prefer --keyring-password-file/-p on setup commands)")
	cmd.ValidArgsFunction = completeKeyNames()
	return cmd
}

func newKeysImportCmd() *cobra.Command {
	var keyringPasswordFile string
	var keyringPasswordStdin bool
	var keyringPasswordExec string

	cmd := &cobra.Command{
		Use:   "import <name>",
		Short: "Import key from armored JSON",
		Long:  "Import a key from armored JSON. You will be prompted for the keyring password.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pw, err := resolveKeyringPassword(cmd, false)
			if err != nil {
				return err
			}
			var input io.Reader
			if pw != "" {
				input = strings.NewReader(pw + "\n")
			} else {
				uio.LogInfo("Enter keyring password (input is hidden):")
			}
			return execKeys(input, "import", args[0])
		},
	}

	f := cmd.Flags()
	f.StringVarP(&keyringPasswordFile, "keyring-password-file", "p", "", "Read keyring password from file (alias: -p)")
	f.BoolVar(&keyringPasswordStdin, "keyring-password-stdin", false, "Read keyring password from stdin")
	f.StringVar(&keyringPasswordExec, "keyring-password-exec", "", "Execute command and read keyring password from stdout")

	return cmd
}

func newKeysDeleteCmd() *cobra.Command {
	var yes bool
	var keyringPass string
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a key from keyring",
		Long:  "Delete a key from the keyring. This is irreversible!",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uio.LogWarning("This will permanently delete key %q", args[0])
			ok, err := uio.Confirm(fmt.Sprintf("Permanently delete key %q from the keyring?", args[0]), yes)
			if err != nil {
				return err
			}
			if !ok {
				uio.LogInfo("Aborted.")
				return nil
			}
			// file keyring is encrypted: open it with the keyring passphrase.
			return execKeys(strings.NewReader(keyringPass+"\n"), "delete", args[0], "--yes")
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().StringVar(&keyringPass, "keyring-pass", "", "Keyring password (inline; for CI prefer --keyring-password-file/-p on setup commands)")
	cmd.ValidArgsFunction = completeKeyNames()
	return cmd
}

func execKeys(input io.Reader, args ...string) error {
	out, err := execKeysRaw(input, args...)
	if err != nil {
		return err
	}
	if len(out) > 0 {
		uio.Print(strings.TrimSpace(string(out)))
	}
	return nil
}

// unmarshalKeysJSON parses the JSON emitted by `umeshnode keys list --output json`
// into a generic slice (so we don't pin the cosmos-sdk types here).
func unmarshalKeysJSON(out []byte) ([]map[string]any, error) {
	var keys []map[string]any
	if err := json.Unmarshal(out, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// execKeysRaw runs `umeshd keys <args>` inside the container and returns the
// raw stdout. Used by both execKeys and the key-name shell completion.
func execKeysRaw(input io.Reader, args ...string) ([]byte, error) {
	docker := dkrcmd.New(
		dkrcmd.WithImage(global.Image),
		dkrcmd.WithHome(global.Home),
		dkrcmd.WithDataDir(global.DataDir),
		dkrcmd.WithBackupsDir(nodeinit.BackupsDir(global.DataDir)),
	)
	fullArgs := []string{"umeshnode", "keys",
		"--keyring-backend", "file",
		"--keyring-dir", global.Home + "/keyring",
	}
	fullArgs = append(fullArgs, args...)
	out, err := docker.RunMount(input, fullArgs...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
