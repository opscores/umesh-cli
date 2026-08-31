package uio

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/term"
)

// ReadSecret prompts for a password on the controlling terminal without
// echoing input. Returns an error if no TTY is available.
func ReadSecret(prompt string) (string, error) {
	if !term.IsTerminal(int(syscall.Stdin)) {
		return "", fmt.Errorf("no TTY available for secret input; pass the value via a flag or env")
	}
	_, _ = fmt.Fprint(os.Stdout, prompt+" ")
	b, err := term.ReadPassword(int(syscall.Stdin))
	_, _ = fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
