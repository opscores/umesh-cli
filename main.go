package main

import (
	"fmt"
	"os"

	"github.com/opscores/umesh-cli/cmd"
)

func main() {
	rootCmd := cmd.NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "umeshctl:", err)
		os.Exit(1)
	}
}
