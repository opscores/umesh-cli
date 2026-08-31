package uio

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// Confirm asks the operator to confirm a destructive operation on an
// interactive terminal. Returns true when:
//   - a non-interactive session (no TTY) — callers decide behavior, so
//     Confirm returns false unless alreadyApproved is set;
//   - the operator answers y/yes (any case) to the prompt;
//   - alreadyApproved is true (--yes / --force passed).
//
// alreadyApproved short-circuits without prompting, so scripts can pass
// --yes/-y to run non-interactively.
func Confirm(prompt string, alreadyApproved bool) (bool, error) {
	if alreadyApproved {
		return true, nil
	}
	if !term.IsTerminal(int(syscall.Stdin)) || !term.IsTerminal(int(syscall.Stdout)) {
		return false, nil
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return ans == "y" || ans == "yes", nil
}