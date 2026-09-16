// Command exhale is the EXALTED Terminal Bible learning tool. Running it with
// no arguments starts the interactive terminal UI; subcommands provide the
// scripting / import / server interface.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gigatone/biblelearn/internal/engine"
)

// version is injected at build time via -ldflags "-X main.version=$(VERSION)".
var version = "dev"

const usage = `EXALTED Terminal — offline Bible learning & study tool

Usage:
  exalted [command]

Commands:
  (no arg)          Start the interactive terminal UI
  versions          List installed versions
  install-pdf       Import a Bible version from a folder/zip of book PDFs
  install-text      Import a Bible version from USFM or plain text
  fetch-archive     Search & install a public-domain Bible from the Internet Archive
  install-nkjv      Install the New King James Version (stock default, GetBible API)
  install-nkjv-pdf  Install the New King James Version from a whole-Bible PDF file
  plugin            Manage installable plugins (list/install/uninstall)
  church            Church locator plugin: find nearby churches (open-source OSM)
  read REF          Print a passage, e.g. "John 3:16", "Gen 1"
  search QUERY      Full-text search across installed versions
  terms PREFIX      Suggest/autocomplete terms for the search box
  index             Rebuild the term & search index for installed versions
  notes             Manage study notes (trash is recoverable)
  versions          List versions; rm/restore/purge trash a version
  bridge            Cyberpunk 2077 CET companion: watch a mod folder and
                    answer JSON commands against an isolated offline store
  backup            Create a flash-safe, fsynced snapshot of your data
  serve             Serve the local web/app UI
  version           Print the Exalted version
  help              Show this help

Global flags:
  --data DIR        Data directory (default ~/.biblelearn)
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		runTUI("")
		return
	}
	// Parse the global --data DIR flag out of the arg list.
	dataDir := ""
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--data" && i+1 < len(args):
			dataDir = args[i+1]
			i++
		case strings.HasPrefix(a, "--data="):
			dataDir = strings.TrimPrefix(a, "--data=")
		default:
			rest = append(rest, a)
		}
	}

	cmd := ""
	if len(rest) > 0 {
		cmd = rest[0]
		rest = rest[1:]
	}
	ctx := context.Background()

	switch cmd {
	case "", "tui", "ui":
		runTUI(dataDir)
	case "help", "-h", "--help":
		fmt.Print(usage)
	case "versions":
		runVersions(dataDir, rest)
	case "install-pdf":
		runInstallPDF(ctx, dataDir, rest)
	case "install-text":
		runInstallText(ctx, dataDir, rest)
	case "fetch-archive":
		runFetchArchive(ctx, dataDir, rest)
	case "install-nkjv":
		runInstallNKJV(ctx, dataDir, rest)
	case "install-nkjv-pdf":
		runInstallNKJVPdf(ctx, dataDir, rest)
	case "plugin":
		runPlugin(ctx, dataDir, rest)
	case "church", "church-locator":
		runChurch(ctx, dataDir, rest)
	case "read", "get":
		runRead(dataDir, rest)
	case "search":
		runSearch(dataDir, rest)
	case "terms":
		runTerms(dataDir, rest)
	case "index":
		runIndex(dataDir, rest)
	case "notes":
		runNotes(dataDir, rest)
	case "backup":
		runBackup(dataDir, rest)
	case "version", "--version", "-V":
		fmt.Printf("EXALTED Terminal %s\n", version)
	case "serve":
		runServe(dataDir, rest)
	case "bridge":
		runBridge(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

func openEngine(dataDir string) (*engine.Engine, error) {
	e, err := engine.New(dataDir)
	if err != nil {
		return nil, err
	}
	// Seed the embedded public-domain KJV stock standard (idempotent, offline).
	if err := e.EnsureDefaultVersion(context.Background()); err != nil {
		return nil, err
	}
	return e, nil
}
