package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gigatone/biblelearn/internal/backup"
	"github.com/gigatone/biblelearn/internal/bibleimport"
	"github.com/gigatone/biblelearn/internal/engine"
	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/store"
	"github.com/gigatone/biblelearn/internal/textclean"
)

func runVersions(dataDir string, args []string) {
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	if len(args) > 0 {
		switch args[0] {
		case "trash":
			vs, err := e.TrashedVersions()
			must(err)
			if len(vs) == 0 {
				fmt.Println("Nothing in the version trash.")
				return
			}
			fmt.Printf("%-8s %-28s %s\n", "ID", "Name", "Source")
			for _, v := range vs {
				fmt.Printf("%-8s %-28s %s\n", v.ID, trunc(v.Name, 28), v.Source)
			}
			fmt.Println("\nrestore: exalted versions restore <ID>   purge: exalted versions purge <ID>")
			return
		case "restore":
			if len(args) < 2 {
				fatal("usage: exalted versions restore <ID>")
			}
			must(e.RestoreVersion(args[1]))
			fmt.Printf("Restored version %q.\n", args[1])
			return
		case "purge":
			if len(args) < 2 {
				fatal("usage: exalted versions purge <ID>")
			}
			must(e.PurgeVersion(args[1]))
			fmt.Printf("Permanently purged version %q.\n", args[1])
			return
		case "rm", "remove", "uninstall":
			if len(args) < 2 {
				fatal("usage: exalted versions rm <ID>")
			}
			must(e.RemoveVersion(args[1]))
			fmt.Printf("Moved version %q to trash (restore: exalted versions restore %s).\n", args[1], args[1])
			return
		default:
			fatal("usage: exalted versions [trash | restore <ID> | purge <ID> | rm <ID>]")
		}
	}
	vs, err := e.ListVersions()
	must(err)
	active, _ := e.ActiveVersionID()
	if len(vs) == 0 {
		fmt.Println("No versions installed. Use 'exalted install-pdf' to add one.")
		return
	}
	fmt.Printf("%-8s %-28s %-6s %-9s %-6s %s\n", "ID", "Name", "Lang", "Col", "Books", "Source")
	for _, v := range vs {
		marker := " "
		if v.ID == active {
			marker = "*"
		}
		coll := v.Collection
		if coll == "" {
			coll = "bible"
		}
		present := v.BooksPresent
		fmt.Printf("%s%-7s %-28s %-6s %-9s %3d    %s\n",
			marker, v.ID, trunc(v.Name, 28), v.Lang, coll, present, v.Source)
	}
	fmt.Println("\n* = active version")
}

func runInstallPDF(ctx context.Context, dataDir string, args []string) {
	fs := flag.NewFlagSet("install-pdf", flag.ExitOnError)
	id := fs.String("id", "", "version ID, e.g. KJV")
	name := fs.String("name", "", "version name, e.g. King James Version")
	lang := fs.String("lang", "en", "BCP-47 language code")
	coll := fs.String("collection", "bible", "collection: bible or egw")
	source := fs.String("source", "user PDF import", "source description")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal("usage: exalted install-pdf --id KJV --name \"...\" <dir-or-zip>")
	}
	if *id == "" {
		fatal("--id is required")
	}
	path := fs.Arg(0)
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	v := model.Version{ID: *id, Name: *name, Lang: *lang, Collection: *coll, Source: *source, Enabled: true}
	opt := e.PDFOption(func(book string, verses int) {
		fmt.Printf("  %-34s %6d verses\n", book, verses)
	})
	var res engine.PDFInstallResult
	if isZip(path) {
		res, err = e.InstallPDFZip(ctx, v, path, opt)
	} else {
		res, err = e.InstallPDFDir(ctx, v, path, opt)
	}
	must(err)
	fmt.Printf("\nImported %d verses into version %q.\n", res.Verses, *id)
	if len(res.Skipped) > 0 {
		fmt.Printf("Books skipped (failed verification, not imported):\n")
		for _, s := range res.Skipped {
			fmt.Printf("  - %s\n", s)
		}
		fmt.Println("You can re-run to retry; nothing is registered unless at least one book parsed.")
	}
}

func runInstallText(ctx context.Context, dataDir string, args []string) {
	fs := flag.NewFlagSet("install-text", flag.ExitOnError)
	id := fs.String("id", "", "version ID, e.g. NKJV")
	name := fs.String("name", "", "version name")
	lang := fs.String("lang", "en", "BCP-47 language code")
	coll := fs.String("collection", "bible", "collection: bible or egw")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal("usage: exalted install-text --id NKJV --name \"...\" <file>")
	}
	if *id == "" {
		fatal("--id is required")
	}
	data, err := os.ReadFile(fs.Arg(0))
	must(err)
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	v := model.Version{ID: *id, Name: *name, Lang: *lang, Collection: *coll, Source: "user file import", Enabled: true}
	n, err := e.InstallVersionText(ctx, v, string(data))
	must(err)
	fmt.Printf("Imported %d verses into version %q.\n", n, *id)
}

func runFetchArchive(ctx context.Context, dataDir string, args []string) {
	fs := flag.NewFlagSet("fetch-archive", flag.ExitOnError)
	query := fs.String("q", "", "search query (e.g. KJV bible)")
	list := fs.Bool("list", false, "list search results and exit")
	id := fs.String("id", "", "version ID to install under")
	name := fs.String("name", "", "version name")
	item := fs.String("item", "", "Internet Archive item identifier to install directly")
	fs.Parse(args)
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	if *item != "" {
		if *id == "" {
			fatal("--id is required with --item")
		}
		v := model.Version{ID: *id, Name: *name, Collection: "bible", Source: "Internet Archive", SourceURL: "https://archive.org/details/" + *item, Enabled: true}
		n, err := e.InstallFromArchiveItem(ctx, v, *item, func(p bibleimport.Progress) {
			if p.Book != "" {
				fmt.Printf("  %-34s %6d verses\n", p.Book, p.VersesAdded)
			}
		})
		must(err)
		fmt.Printf("\nImported %d verses from archive item %q.\n", n, *item)
		return
	}
	if *query == "" {
		fatal("usage: exalted fetch-archive --q \"kjv bible\" | --item <id>")
	}
	items, err := e.FetchFromArchive(ctx, *query, 8)
	must(err)
	if len(items) == 0 {
		fmt.Println("No items found.")
		return
	}
	for i, it := range items {
		desc := strings.ReplaceAll(it.Description, "\n", " ")
		if len(desc) > 70 {
			desc = desc[:70] + "…"
		}
		fmt.Printf("%d) %s  —  %s\n   %s\n", i+1, it.Identifier, it.Title, desc)
	}
	if *list {
		return
	}
	fmt.Println("\nInstall one with: exalted fetch-archive --item <identifier> --id <ID> --name \"<Name>\"")
}

func runInstallNKJV(ctx context.Context, dataDir string, args []string) {
	fs := flag.NewFlagSet("install-nkjv", flag.ExitOnError)
	fs.Parse(args)
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	if _, ok, _ := e.Store.GetVersion("NKJV"); ok {
		fmt.Println("NKJV is already installed.")
		return
	}
	v := model.Version{ID: "NKJV", Name: "New King James Version", Lang: "en",
		Collection: "bible", Source: "GetBible", SourceURL: "https://bolls.life", Enabled: true}
	fmt.Println("Installing: New King James Version (GetBible API)...")
	n, err := e.InstallNKJV(ctx, v, func(book string, verses int) {
		fmt.Printf("  %-34s %6d verses\n", book, verses)
	})
	must(err)
	fmt.Printf("\nImported %d NKJV verses.\n", n)
	if n == 0 {
		fmt.Println("Warning: nothing fetched — check internet connection and retry.")
		return
	}
	if err := e.SetActiveVersion("NKJV"); err == nil {
		fmt.Println("NKJV set as the active default version.")
	}
}

// runInstallNKJVPdf installs NKJV from a single whole-Bible PDF by splitting it
// into per-book segments (two-column NKJV layout) and verifying each book's
// verse count.
func runInstallNKJVPdf(ctx context.Context, dataDir string, args []string) {
	fs := flag.NewFlagSet("install-nkjv-pdf", flag.ExitOnError)
	fs.Parse(args)
	path := strings.Join(fs.Args(), " ")
	if path == "" {
		fatal("usage: exalted install-nkjv-pdf <path-to-whole-bible-pdf>")
	}
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	v := model.Version{ID: "NKJV", Name: "New King James Version", Lang: "en",
		Collection: "bible", Source: "PDF (user-provided)", Enabled: true}
	fmt.Println("Importing NKJV from whole-Bible PDF...")
	n, skipped, err := e.InstallNKJVPdf(ctx, v, path, func(book string, verses int) {
		fmt.Printf("  %-34s %6d verses\n", book, verses)
	})
	must(err)
	fmt.Printf("\nImported %d NKJV verses across the books that passed verification.\n", n)
	if len(skipped) > 0 {
		fmt.Printf("Skipped %d book(s) that failed verse-count verification: %v\n", len(skipped), skipped)
	}
	if n == 0 {
		return
	}
	if err := e.SetActiveVersion("NKJV"); err == nil {
		fmt.Println("NKJV set as the active default version.")
	}
}

func runPlugin(ctx context.Context, dataDir string, args []string) {
	fs := flag.NewFlagSet("plugin", flag.ExitOnError)
	fs.Parse(args)
	args = fs.Args()
	if len(args) == 0 {
		fatal("usage: exalted plugin (list | install <name> | uninstall <name>)")
	}
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	switch args[0] {
	case "list":
		reg := e.Plugins()
		for _, p := range reg.List() {
			inst, _ := e.PluginInstalled(p.Name)
			mark := " "
			if inst {
				mark = "*"
			}
			fmt.Printf("%s %s v%s  (%s)\n    %s\n", mark, p.DisplayName, p.Version, p.Name, p.Description)
		}
	case "install":
		if len(args) < 2 {
			fatal("usage: exalted plugin install <name>")
		}
		must(e.InstallPlugin(ctx, args[1]))
		fmt.Printf("Installed plugin %q.\n", args[1])
	case "uninstall":
		if len(args) < 2 {
			fatal("usage: exalted plugin uninstall <name>")
		}
		must(e.UninstallPlugin(ctx, args[1]))
		fmt.Printf("Uninstalled plugin %q.\n", args[1])
	default:
		fatal("unknown plugin subcommand")
	}
}

func runChurch(ctx context.Context, dataDir string, args []string) {
	radius := 8.0
	var queryParts []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--radius" && i+1 < len(args):
			if v, err := strconv.ParseFloat(args[i+1], 64); err == nil {
				radius = v
			}
			i++
		case strings.HasPrefix(args[i], "--radius="):
			if v, err := strconv.ParseFloat(strings.TrimPrefix(args[i], "--radius="), 64); err == nil {
				radius = v
			}
		default:
			queryParts = append(queryParts, args[i])
		}
	}
	query := strings.Join(queryParts, " ")
	if query == "" {
		fatal("usage: exalted church \"<place, city or address>\" [--radius KM]")
	}
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	place, churches, err := e.LocateChurches(ctx, query, radius)
	must(err)
	fmt.Printf("Churches near %s (within %.0f km):\n\n", place, radius)
	for i, c := range churches {
		name := c.Name
		if name == "" {
			name = "(unnamed)"
		}
		fmt.Printf("%d. %s\n   Denomination: %s\n   Address: %s\n   Distance: %.2f km\n\n",
			i+1, name, c.Denom, c.Address, c.DistanceKm)
	}
}

func runRead(dataDir string, args []string) {
	if len(args) < 1 {
		fatal("usage: exalted read <REF> [--version ID]")
	}
	refSpec := args[0]
	versionID := ""
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--version" {
			versionID = args[i+1]
		}
	}
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	if versionID == "" {
		versionID, _ = e.ActiveVersionID()
	}
	col := "bible"
	versioned, r, ok := model.ParseRefFull(refSpec)
	if ok {
		if versioned != "" {
			versionID = versioned
		}
		if r.Collection != "" {
			col = r.Collection
		}
	}
	if !ok {
		r, ok = model.ParseRef(refSpec)
		if !ok {
			fatal("could not parse reference: " + refSpec)
		}
	}
	r.VersionID = versionID
	bc, present, err := e.Store.GetBook(versionID, col, r.Book)
	must(err)
	if !present {
		fatal(fmt.Sprintf("version %q has no text for book %d", versionID, r.Book))
	}
	if r.Verse == 0 {
		ch, ok := bc.Chapters[r.Chapter]
		if !ok {
			fatal("chapter not found")
		}
		fmt.Printf("%s %d  (%s)\n\n", r.BookName(), r.Chapter, versionID)
		for _, v := range ch.Order {
			fmt.Printf("%3d  %s\n", v, textclean.HTML(ch.Verses[v]))
		}
		return
	}
	txt, ok, err := e.Store.VerseText(versionID, r)
	must(err)
	if !ok {
		fatal("verse not found")
	}
	fmt.Printf("%s %d:%d  (%s)\n\n%s\n", r.BookName(), r.Chapter, r.Verse, versionID, txt)
}

func runSearch(dataDir string, args []string) {
	if len(args) < 1 {
		fatal("usage: exalted search <query> [--version ID]")
	}
	query := args[0]
	versionID := ""
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--version" {
			versionID = args[i+1]
		}
	}
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	res, err := e.Store.Search(query, versionID, "", 40)
	must(err)
	if len(res) == 0 {
		fmt.Println("No results.")
		return
	}
	for _, r := range res {
		ref := model.Ref{VersionID: r.VersionID, Collection: r.Collection, Book: r.Book, Chapter: r.Chapter, Verse: r.Verse}
		fmt.Printf("%-22s  %s\n", ref.String(), trunc(r.Text, 80))
	}
}

func runTerms(dataDir string, args []string) {
	if len(args) < 1 {
		fatal("usage: exalted terms <prefix> [--version ID] [--alpha]")
	}
	prefix := args[0]
	versionID := ""
	freq := true
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--version":
			versionID = args[i+1]
		case "--alpha":
			freq = false
		}
	}
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	sug, err := e.SuggestTerms(prefix, versionID, "", 30, freq)
	must(err)
	if len(sug) == 0 {
		fmt.Println("No matching terms.")
		return
	}
	if freq {
		for _, s := range sug {
			fmt.Printf("%-28s %8d\n", s.Term, s.Count)
		}
	} else {
		for _, s := range sug {
			fmt.Printf("%s\n", s.Term)
		}
	}
}

func runIndex(dataDir string, args []string) {
	versionID := ""
	if len(args) > 0 && args[0] != "" {
		versionID = args[0]
	}
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	vs, err := e.ListVersions()
	must(err)
	targets := []string{}
	if versionID != "" {
		if _, ok, _ := e.Store.GetVersion(versionID); !ok {
			fatal("version not found: " + versionID)
		}
		targets = append(targets, versionID)
	} else {
		for _, v := range vs {
			targets = append(targets, v.ID)
		}
	}
	for _, id := range targets {
		v, _, _ := e.Store.GetVersion(id)
		coll := v.Collection
		if coll == "" {
			coll = "bible"
		}
		verses := 0
		books, _ := e.Store.ListVersionBooks(id, coll)
		for _, ord := range books {
			bc, ok, _ := e.Store.GetBook(id, coll, ord)
			if !ok {
				continue
			}
			for _, ch := range bc.Chapters {
				if ch != nil {
					verses += len(ch.Verses)
				}
			}
		}
		fmt.Printf("Indexing %s (%s, %d verses)...\n", id, coll, verses)
		if err := e.Store.RebuildTermIndex(id, coll); err != nil {
			fmt.Fprintf(os.Stderr, "  term index: %v\n", err)
		}
		fmt.Printf("  term index rebuilt\n")
	}
	fmt.Println("done.")
}

func runNotes(dataDir string, args []string) {
	if len(args) < 1 {
		fatal("usage: exalted notes list|add|rm|trash|restore|purge")
	}
	e, err := openEngine(dataDir)
	must(err)
	defer e.Close()
	versionID, _ := e.ActiveVersionID()
	switch args[0] {
	case "list":
		ns, err := e.Store.ListNotes(versionID)
		must(err)
		for _, n := range ns {
			ref := model.Ref{VersionID: versionID, Collection: "bible", Book: n.Book, Chapter: n.Chapter, Verse: n.Verse}
			fmt.Printf("%s\n  %s\n", ref.String(), trunc(n.Body, 100))
		}
	case "add", "rm":
		if len(args) < 2 {
			fatal("usage: exalted notes add|rm <REF> [body]")
		}
		r, ok := model.ParseRef(args[1])
		if !ok {
			fatal("bad reference")
		}
		body := ""
		if args[0] == "add" {
			body = strings.Join(args[2:], " ")
			if body == "" {
				fatal("note body required for add")
			}
		}
		note := store.Note{VersionID: versionID, Book: r.Book, Chapter: r.Chapter, Verse: r.Verse, Body: body}
		must(e.Store.SaveNote(note))
		fmt.Println("saved")
	case "trash":
		ns, err := e.Store.ListTrashedNotes(versionID)
		must(err)
		if len(ns) == 0 {
			fmt.Println("Nothing in the note trash.")
			return
		}
		for _, n := range ns {
			ref := model.Ref{VersionID: versionID, Collection: "bible", Book: n.Book, Chapter: n.Chapter, Verse: n.Verse}
			fmt.Printf("%s\n  %s\n", ref.String(), trunc(n.Body, 100))
		}
		fmt.Println("\nrestore: exalted notes restore <REF>   purge: exalted notes purge")
	case "restore":
		if len(args) < 2 {
			fatal("usage: exalted notes restore <REF>")
		}
		r, ok := model.ParseRef(args[1])
		if !ok {
			fatal("bad reference")
		}
		restored, err := e.Store.RestoreNote(versionID, r.Book, r.Chapter, r.Verse)
		must(err)
		if restored {
			fmt.Println("restored")
		} else {
			fmt.Println("no trashed note at that reference")
		}
	case "purge":
		n, err := e.Store.PurgeTrashedNotes("")
		must(err)
		fmt.Printf("purged %d trashed note(s)\n", n)
	default:
		fatal("usage: exalted notes list|add|rm|trash|restore|purge")
	}
}

// --- backup ---

func runBackup(dataDir string, args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	to := fs.String("to", "", "destination directory for backups (default ~/biblelearn-backups)")
	fs.Parse(args)
	destRoot := *to
	if destRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fatal(err.Error())
		}
		destRoot = filepath.Join(home, "biblelearn-backups")
	}
	s, err := store.Open(dataDir)
	must(err)
	defer s.Close()
	dest, err := backup.Snapshot(dataDir, destRoot, s.Checkpoint)
	must(err)
	fmt.Printf("Backup created: %s\n", dest)
}

// --- small helpers ---

func must(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func isZip(p string) bool {
	return strings.EqualFold(filepath.Ext(p), ".zip")
}
