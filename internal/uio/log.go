package uio

import (
	"fmt"
	"os"
	"sync"
)

var (
	verbose bool
	logMu   sync.Mutex
)

// SetVerbose controls whether debug-level lines are printed.
func SetVerbose(v bool) { verbose = v }

// LogError prints a red [ERROR] line.
func LogError(format string, a ...any) {
	if IsQuiet() {
		return
	}
	c := Colors()
	fmt.Fprintf(os.Stderr, "%s%s[ERROR]%s %s\n", c.Red, c.Bold, c.NC, fmt.Sprintf(format, a...))
}

// Fatal logs a red [ERROR] line and exits the process with status 1.
// Use it for terminal errors that must abort the command immediately without
// being re-printed by main (main.go prints returned errors from RunE).
func Fatal(err error) {
	if err == nil {
		return
	}
	LogError("%s", err)
	os.Exit(1)
}

// LogWarning prints a yellow [WARNING] line.
func LogWarning(format string, a ...any) {
	if IsQuiet() {
		return
	}
	c := Colors()
	fmt.Fprintf(os.Stderr, "%s[WARNING]%s %s\n", c.Yellow, c.NC, fmt.Sprintf(format, a...))
}

// LogInfo prints a blue [INFO] line.
func LogInfo(format string, a ...any) {
	if IsQuiet() {
		return
	}
	c := Colors()
	_, _ = fmt.Fprintf(os.Stdout, "%s[INFO]%s %s\n", c.Blue, c.NC, fmt.Sprintf(format, a...))
}

// LogSuccess prints a green [OK] line.
func LogSuccess(format string, a ...any) {
	if IsQuiet() {
		return
	}
	c := Colors()
	_, _ = fmt.Fprintf(os.Stdout, "%s[OK]%s %s\n", c.Green, c.NC, fmt.Sprintf(format, a...))
}

// LogStep prints a cyan >>> step line.
func LogStep(format string, a ...any) {
	if IsQuiet() {
		return
	}
	c := Colors()
	_, _ = fmt.Fprintf(os.Stdout, "%s%s>>> %s%s\n", c.Cyan, c.Bold, fmt.Sprintf(format, a...), c.NC)
}

// LogDebug prints a line only when --verbose is set.
func LogDebug(format string, a ...any) {
	if !verbose {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", fmt.Sprintf(format, a...))
}

// Print prints a plain line to stdout.
func Print(format string, a ...any) {
	if IsQuiet() {
		return
	}
	_, _ = fmt.Fprintf(os.Stdout, format+"\n", a...)
}
