package uio

import (
	"os"
	"sync"
)

var (
	colorsEnabled bool
	noColor       bool
	quiet         bool
	colorsOnce    sync.Once
)

// SetColorMode sets whether color output is disabled (--no-color flag or NO_COLOR env).
func SetColorMode(disable bool) { noColor = disable }

// SetQuiet sets whether non-essential output should be suppressed.
func SetQuiet(q bool) { quiet = q }

// IsQuiet returns whether quiet mode is enabled.
func IsQuiet() bool { return quiet }

// Colors returns ANSI color codes for human output. Codes are empty when the
// output is not a TTY, mirroring scripts/common/colors.sh behavior.
func Colors() struct{ Red, Green, Yellow, Blue, Magenta, Cyan, Bold, NC string } {
	colorsOnce.Do(func() {
		fi, err := os.Stdout.Stat()
		colorsEnabled = err == nil && fi.Mode()&os.ModeCharDevice != 0
	})
	var c struct{ Red, Green, Yellow, Blue, Magenta, Cyan, Bold, NC string }
	if !colorsEnabled || noColor {
		return c
	}
	c.Red = "\x1b[31m"
	c.Green = "\x1b[32m"
	c.Yellow = "\x1b[33m"
	c.Blue = "\x1b[34m"
	c.Magenta = "\x1b[35m"
	c.Cyan = "\x1b[36m"
	c.Bold = "\x1b[1m"
	c.NC = "\x1b[0m"
	return c
}
