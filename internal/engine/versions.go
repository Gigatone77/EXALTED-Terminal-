package engine

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gigatone/biblelearn/internal/bibleimport"
	"github.com/gigatone/biblelearn/internal/bundled"
	"github.com/gigatone/biblelearn/internal/collections"
	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/nkjv"
	"github.com/gigatone/biblelearn/internal/online"
	"github.com/gigatone/biblelearn/internal/pdfimport"
	"github.com/gigatone/biblelearn/internal/store"
)

// the setting key storing the user's active version.
const ActiveVersionKey = "active_version"

// VersionInfo enriches a registered version with its local completeness.
type VersionInfo struct {
	model.Version
	// BooksPresent is the number of canonical books currently stored locally.
	BooksPresent int
	// VersesStored is an estimate of verses present across all stored books.
	VersesStored int
	// Complete is true when all 66 books are present locally.
	Complete bool
}

// ListVersions returns the registered versions with completeness info.
func (e *Engine) ListVersions() ([]VersionInfo, error) {
	vs, err := e.Store.ListVersions()
	if err != nil {
		return nil, err
	}
	var out []VersionInfo
	for _, v := range vs {
		coll := v.Collection
		if coll == "" {
			coll = "bible"
		}
		clist := collections.ByIDSafe(coll)
		totalBooks := len(clist)
		present, _ := e.Store.ImportedBooks(v.ID, coll)
		vi := VersionInfo{Version: v, BooksPresent: len(present), Complete: len(present) >= totalBooks}
		if len(present) > 0 {
			for ord := range present {
				if n, err := versionVerses(e, v.ID, coll, ord); err == nil {
					vi.VersesStored += n
				}
			}
		}
		out = append(out, vi)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func versionVerses(e *Engine, id, coll string, ord int) (int, error) {
	bc, ok, err := e.Store.GetBook(id, coll, ord)
	if err != nil || !ok {
		return 0, err
	}
	n := 0
	for _, ch := range bc.Chapters {
		if ch != nil {
			n += len(ch.Verses)
		}
	}
	return n, nil
}

// ActiveVersionID returns the current active version. It defaults to NKJV
// (the stock dominant translation) when installed, falling back to KJV (the
// bundled public-domain stock standard), then to any available version.
func (e *Engine) ActiveVersionID() (string, error) {
	id, err := e.Store.GetSetting(ActiveVersionKey)
	if err != nil {
		return "", err
	}
	if id != "" {
		if _, ok, _ := e.Store.GetVersion(id); ok {
			return id, nil
		}
	}
	// No valid active version: pick the stock default, preferring NKJV then KJV.
	for _, cand := range []string{"NKJV", "KJV"} {
		if _, ok, _ := e.Store.GetVersion(cand); ok {
			_ = e.Store.SetSetting(ActiveVersionKey, cand)
			return cand, nil
		}
	}
	ids, _ := e.AvailableVersions()
	if len(ids) > 0 {
		_ = e.Store.SetSetting(ActiveVersionKey, ids[0])
		return ids[0], nil
	}
	_ = e.Store.SetSetting(ActiveVersionKey, "")
	return "", nil
}

// SetActiveVersion sets the active version if it exists.
func (e *Engine) SetActiveVersion(id string) error {
	if _, ok, err := e.Store.GetVersion(id); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("version %q not installed", id)
	}
	return e.Store.SetSetting(ActiveVersionKey, id)
}

// AvailableVersions returns IDs of installed versions that have at least one
// book present.
func (e *Engine) AvailableVersions() ([]string, error) {
	vs, err := e.ListVersions()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range vs {
		if v.BooksPresent > 0 {
			out = append(out, v.ID)
		}
	}
	return out, nil
}

// InstallVersionText registers a version and imports its text from a string
// (USFM or plain text, auto-detected), returning verses imported. The version
// is only registered when at least one verse was successfully parsed, so a bad
// file can never leave an empty phantom version behind.
func (e *Engine) InstallVersionText(ctx context.Context, v model.Version, content string) (int, error) {
	if v.ID == "" {
		return 0, fmt.Errorf("version ID required")
	}
	coll := v.Collection
	if coll == "" {
		coll = "bible"
	}
	if strings.TrimSpace(content) == "" {
		return 0, fmt.Errorf("no content to import")
	}
	opt := bibleimport.ImportOptions{Collection: coll}
	var n int
	var err error
	if bibleimport.IsUSFM(content) {
		n, err = bibleimport.ImportUSFMText(e.Store, v.ID, content, opt)
	} else {
		n, err = bibleimport.ImportPlainText(e.Store, v.ID, content, opt)
	}
	if err == nil && n == 0 {
		return 0, fmt.Errorf("no verses recognized — not a Bible text file?")
	}
	if err != nil {
		return n, err
	}
	if err := e.Store.RegisterVersion(v); err != nil {
		return n, err
	}
	_ = e.RebuildTerms(v.ID, coll)
	return n, nil
}

// RemoveVersion moves a version to the recoverable trash (nothing is deleted:
// the archive is relocated under .trash/versions and the registry row is
// soft-deleted). Use RestoreVersion to bring it back.
func (e *Engine) RemoveVersion(id string) error {
	v, ok, _ := e.Store.GetVersion(id)
	if !ok {
		return fmt.Errorf("version %q not installed", id)
	}
	if v.Builtin {
		return fmt.Errorf("cannot remove built-in version %q", id)
	}
	active, _ := e.ActiveVersionID()
	if err := e.Store.RemoveVersion(id); err != nil {
		return err
	}
	if active == id {
		// fall back to any remaining version.
		ids, _ := e.AvailableVersions()
		if len(ids) > 0 {
			_ = e.Store.SetSetting(ActiveVersionKey, ids[0])
		} else {
			_ = e.Store.SetSetting(ActiveVersionKey, "")
		}
	}
	return nil
}

// TrashedVersions lists versions currently in the recoverable trash.
func (e *Engine) TrashedVersions() ([]model.Version, error) {
	return e.Store.TrashedVersions()
}

// RestoreVersion moves a trashed version back into the active installs.
func (e *Engine) RestoreVersion(id string) error {
	return e.Store.RestoreVersion(id)
}

// PurgeVersion permanently deletes a trashed version. Built-in versions are
// protected.
func (e *Engine) PurgeVersion(id string) error {
	if v, ok, _ := e.Store.GetVersion(id); ok && v.Builtin {
		return fmt.Errorf("cannot purge built-in version %q", id)
	}
	return e.Store.PurgeVersion(id)
}

// FetchFromArchive finds candidate Bible items on the Internet Archive.
func (e *Engine) FetchFromArchive(ctx context.Context, query string, limit int) ([]online.Item, error) {
	c := online.NewClient()
	return c.SearchBibles(ctx, query, limit)
}

// InstallFromArchiveItem downloads and installs a text Bible from an
// Internet Archive item. Returns number of verses imported.
func (e *Engine) InstallFromArchiveItem(ctx context.Context, v model.Version, itemID string, progress func(bibleimport.Progress)) (int, error) {
	c := online.NewClient()
	meta, err := c.ItemMetadata(ctx, itemID)
	if err != nil {
		return 0, err
	}
	files := meta.PreferTextFiles(meta.Files)
	if len(files) == 0 {
		return 0, fmt.Errorf("no text files found in archive item")
	}
	coll := v.Collection
	if coll == "" {
		coll = "bible"
	}
	if err := e.Store.RegisterVersion(v); err != nil {
		return 0, err
	}
	opt := bibleimport.ImportOptions{Collection: coll, Progress: progress}
	total := 0
	for _, f := range files {
		data, err := c.DownloadFile(ctx, itemID, f.Name)
		if err != nil {
			continue
		}
		content := string(data)
		var n int
		if bibleimport.IsUSFM(content) {
			n, err = bibleimport.ImportUSFMText(e.Store, v.ID, content, opt)
		} else {
			n, err = bibleimport.ImportPlainText(e.Store, v.ID, content, opt)
		}
		if err == nil {
			total += n
		}
	}
	_ = e.RebuildTerms(v.ID, coll)
	return total, nil
}

// SeedEmbeddedKJV installs the public-domain King James Version from the USFM
// data embedded directly in the binary. It is a plug-and-play stock standard
// that requires no network and works fully offline. The version is registered
// as Builtin (non-removable). Idempotent: no-op if KJV already has books.
func (e *Engine) SeedEmbeddedKJV() (int, error) {
	if vs, err := e.Store.ImportedBooks("KJV", "bible"); err == nil && len(vs) > 0 {
		return 0, nil
	}
	content, err := bundled.EmbeddedKJVUSFM()
	if err != nil {
		return 0, err
	}
	v := model.Version{ID: "KJV", Name: "King James Version", Lang: "en",
		Builtin: true, Enabled: true}
	n, err := e.InstallVersionText(context.Background(), v, content)
	if err != nil {
		return n, err
	}
	_ = e.Store.SetSetting(ActiveVersionKey, "KJV")
	return n, nil
}

// InstallNKJV downloads and installs the New King James Version from the
// reliable GetBible API. It is the dominant stock standard alongside the
// embedded public-domain KJV.
func (e *Engine) InstallNKJV(ctx context.Context, v model.Version, progress func(book string, verses int)) (int, error) {
	total, err := nkjv.InstallNKJV(ctx, e.Store, v, progress)
	if err == nil {
		coll := v.Collection
		if coll == "" {
			coll = "bible"
		}
		_ = e.RebuildTerms(v.ID, coll)
	}
	return total, err
}

// InstallNKJVPdf installs the New King James Version from a single whole-Bible
// PDF (the two-column NKJV layout). Books that fail verse-count verification
// are skipped and reported. Returns the number of successfully stored verses.
func (e *Engine) InstallNKJVPdf(ctx context.Context, v model.Version, pdfPath string, progress func(book string, verses int)) (int, []string, error) {
	coll := v.Collection
	if coll == "" {
		coll = "bible"
	}
	if err := e.Store.RegisterVersion(v); err != nil {
		return 0, nil, err
	}
	res, err := pdfimport.ImportWholeBibleFromPDF(e.Store, v.ID, pdfPath, progress)
	if err != nil {
		return 0, nil, err
	}
	_ = e.RebuildTerms(v.ID, coll)
	return res.Verses, res.Skipped, nil
}

// IsComplete reports whether every book is present for a version's collection.
func (e *Engine) IsComplete(id string) (bool, error) {
	v, ok, err := e.Store.GetVersion(id)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	coll := v.Collection
	if coll == "" {
		coll = "bible"
	}
	present, err := e.Store.ImportedBooks(id, coll)
	if err != nil {
		return false, err
	}
	return len(present) >= len(collections.ByIDSafe(coll)), nil
}

// RebuildTerms rebuilds the term/suggestion index for a version.
func (e *Engine) RebuildTerms(versionID, collection string) error {
	if collection == "" {
		collection = "bible"
	}
	return e.Store.RebuildTermIndex(versionID, collection)
}

// SuggestTerms returns search-box suggestions for a prefix.
func (e *Engine) SuggestTerms(prefix, version, collection string, limit int, freq bool) ([]store.TermSuggestion, error) {
	return e.Store.SuggestTerms(prefix, version, collection, limit, freq)
}

// PDFOption carries a per-book progress callback for PDF installs.
type PDFOption struct {
	// Progress is called with (bookName, versesParsed) per book.
	Progress func(book string, verses int)
}

// PDFOption builds a PDFOption from a progress callback.
func (e *Engine) PDFOption(progress func(book string, verses int)) PDFOption {
	return PDFOption{Progress: progress}
}

// PDFInstallResult reports the outcome of a PDF import, including any books
// whose extraction failed verse verification (so no empty version is ever
// silently registered).
type PDFInstallResult struct {
	// Verses is the number of verses successfully stored.
	Verses int
	// Skipped lists human-readable book labels that failed to parse, e.g.
	// "3 Leviticus (no verses extracted)". Normal files import with no skips.
	Skipped []string
}

// InstallPDFDir installs a version from a directory of per-book PDF files. Each
// file's name must contain a 1-based book number (e.g. "43.pdf", "1 Gen.pdf",
// "book-40.pdf"), which is mapped to the collection's book ordinal. The version
// is registered only when at least one book parsed.
func (e *Engine) InstallPDFDir(ctx context.Context, v model.Version, dir string, opt PDFOption) (PDFInstallResult, error) {
	coll := v.Collection
	if coll == "" {
		coll = "bible"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return PDFInstallResult{}, err
	}
	type job struct {
		ord  int
		path string
	}
	var jobs []job
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
			continue
		}
		if ord, ok := ordinalFromName(name, coll); ok {
			jobs = append(jobs, job{ord: ord, path: filepath.Join(dir, name)})
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ord < jobs[j].ord })
	if len(jobs) == 0 {
		return PDFInstallResult{}, fmt.Errorf("no book PDFs found in %s", dir)
	}
	clist := collections.ByIDSafe(coll)
	res := PDFInstallResult{}
	for _, j := range jobs {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		n, perr := pdfimport.ImportBookFromPDF(e.Store, v.ID, coll, j.ord, j.path,
			func(book string, verses int) {
				if opt.Progress != nil {
					opt.Progress(book, verses)
				}
			})
		if perr != nil || n == 0 {
			name := clist[j.ord-1].Name
			reason := "parse error"
			if perr == nil {
				reason = "no verses extracted"
			}
			res.Skipped = append(res.Skipped, fmt.Sprintf("%d %s (%s)", j.ord, name, reason))
			continue
		}
		res.Verses += n
	}
	if res.Verses == 0 {
		return res, fmt.Errorf("no verses could be extracted from PDFs")
	}
	if err := e.Store.RegisterVersion(v); err != nil {
		return res, err
	}
	_ = e.RebuildTerms(v.ID, coll)
	return res, nil
}

// InstallPDFZip installs a version from a zip of per-book PDFs (e.g. the phone
// Bibles like "English-PDF.zip" with "English-PDF/43.pdf"). The archive is
// validated structurally (entries must be per-book PDFs with a usable number)
// before any version is registered.
func (e *Engine) InstallPDFZip(ctx context.Context, v model.Version, zipPath string, opt PDFOption) (PDFInstallResult, error) {
	coll := v.Collection
	if coll == "" {
		coll = "bible"
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return PDFInstallResult{}, err
	}
	defer zr.Close()
	type job struct {
		ord  int
		data []byte
	}
	var jobs []job
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name
		if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
			continue
		}
		if ord, ok := ordinalFromName(filepath.Base(name), coll); ok {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data := make([]byte, 0, f.UncompressedSize64)
			buf := make([]byte, 1<<20)
			for {
				n, rerr := rc.Read(buf)
				if n > 0 {
					data = append(data, buf[:n]...)
				}
				if rerr != nil {
					break
				}
			}
			rc.Close()
			jobs = append(jobs, job{ord: ord, data: data})
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ord < jobs[j].ord })
	if len(jobs) == 0 {
		return PDFInstallResult{}, fmt.Errorf("no book PDFs found in %s", zipPath)
	}
	// Write PDFs to a temp dir then run the dir importer.
	tmp, err := os.MkdirTemp("", "biblepdf-*")
	if err != nil {
		return PDFInstallResult{}, err
	}
	defer os.RemoveAll(tmp)
	for _, j := range jobs {
		name := filepath.Join(tmp, fmt.Sprintf("%d.pdf", j.ord))
		if err := os.WriteFile(name, j.data, 0o644); err != nil {
			return PDFInstallResult{}, err
		}
	}
	return e.InstallPDFDir(ctx, v, tmp, opt)
}

// ordinalFromName extracts a 1-based book ordinal from a PDF filename within a
// collection, validating against the collection's book list.
func ordinalFromName(name string, collection string) (int, bool) {
	base := strings.ToLower(filepath.Base(name))
	num := extractLeadingNumber(base)
	if num <= 0 {
		return 0, false
	}
	book := collections.ByIDSafe(collection)
	if num < 1 || num > len(book) {
		return 0, false
	}
	return num, true
}

func extractLeadingNumber(s string) int {
	// Accept names like "43.pdf", "1 Gen.pdf", "Gene. 10.pdf".
	var digits strings.Builder
	started := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			started = true
		} else if started {
			break
		}
	}
	if digits.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(digits.String())
	return n
}
