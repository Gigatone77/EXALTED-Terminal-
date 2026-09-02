package main

import (
	"fmt"
	"os"
)

// runServe starts the local web/app UI. The full server is implemented in
// webapp; this placeholder keeps the CLI runnable meanwhile.
func runServe(dataDir string, args []string) {
	port := "8080"
	if len(args) > 0 {
		port = args[0]
	}
	_, err := openEngine(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("EXALTED Terminal web UI not yet wired — run on http://localhost:%s\n", port)
}
