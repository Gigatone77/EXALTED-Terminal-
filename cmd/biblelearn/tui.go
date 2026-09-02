package main

import (
	"fmt"
	"os"
)

// runTUI is the interactive Bubble Tea interface. The full TUI is implemented
// in tui.go; this file hosts the entry point so the CLI can be built and tested
// independently while the TUI is developed.
func runTUI(dataDir string) {
	e, err := openEngine(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer e.Close()
	if err := tuiRun(e); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
