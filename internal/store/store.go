// Package store provides the offline-first persistence layer: a SQLite
// database for metadata, learning/quiz state, and the full-text search index,
// plus a directory-based "version archive" that stores verse text as gzipped
// JSON files (one per book) for easy add/remove/import/export.
package store

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gigatone/biblelearn/internal/collections"
	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/textclean"
	_ "modernc.org/sqlite"
)

// dataDir holds the app's on-disk state. Layout:
//
//	data/
//	  app.db                 SQLite (registry, state, FTS index)
//	  versions/
//	    KJV/
//	      version.json        version metadata
//	      books.zip          (removed; per-book files used instead)
//	    KJV/bk/0001.json.gz   book 1 (Genesis) gzipped chapter map
type Store struct {
	mu     sync.Mutex
	dir    string
	db     *sqlDB
	verDir string
}

// Open opens (creating if needed) the store rooted at dir.
func Open(dir string) (*Store, error) {
	if dir == "" {
		dir = defaultDataDir()
	}
	if err := os.MkdirAll(filepath.Join(dir, "versions"), 0o755); err != nil {
		return nil, err
	}
	db, err := openDB(filepath.Join(dir, "app.db"))
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{dir: dir, db: db, verDir: filepath.Join(dir, "versions")}
	// After repairing a contentless FTS table (or a fresh DB), rebuild the
	// verse index once so full-text search is immediately usable.
	if done, err := db.getSetting("fts.reindexed"); err != nil {
		return nil, err
	} else if done == "" {
		if err := s.ReindexAllBooks(); err != nil {
			return nil, err
		}
		if err := db.setSetting("fts.reindexed", "1"); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Close flushes and closes the store.
func (s *Store) Close() error {
	return s.db.Close()
}

// Dir returns the store root directory.
func (s *Store) Dir() string { return s.dir }

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./biblelearn-data"
	}
	return filepath.Join(home, ".biblelearn")
}

// VersionDir returns the archive directory for a given version ID.
func (s *Store) VersionDir(id string) string {
	return filepath.Join(s.verDir, id)
}

// ---------------------------------------------------------------------------
// Version registry
// ---------------------------------------------------------------------------

// RegisterVersion creates/updates metadata for a version. On first register it
// creates the version archive directory.
func (s *Store) RegisterVersion(v model.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == "" {
		return errors.New("version ID required")
	}
	dir := filepath.Join(s.verDir, v.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Persist sidecar metadata.
	metaPath := filepath.Join(dir, "version.json")
	if err := writeJSONFile(metaPath, v); err != nil {
		return err
	}
	return s.db.upsertVersion(v)
}

// ListVersions returns all registered versions.
func (s *Store) ListVersions() ([]model.Version, error) {
	return s.db.listVersions()
}

// GetVersion returns metadata for one version.
func (s *Store) GetVersion(id string) (model.Version, bool, error) {
	return s.db.getVersion(id)
}

// RemoveVersion moves a version to the recoverable trash: its registry row is
// soft-deleted and its archive directory is relocated under .trash/versions so
// nothing is permanently destroyed. Use RestoreVersion to bring it back or
// PurgeVersion to permanently delete it.
func (s *Store) RemoveVersion(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendVersionToTrash(id)
}

// TrashedVersions lists versions currently in the recoverable trash.
func (s *Store) TrashedVersions() ([]model.Version, error) {
	return s.db.listTrashedVersions()
}

// RestoreVersion brings a trashed version back: its archive directory is
// moved back into place and its registry row is un-deleted.
func (s *Store) RestoreVersion(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.verDir, id)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("version %q is already installed", id)
	}
	rel, ok, err := s.trashVersionDir(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("version %q not found in trash", id)
	}
	if err := os.Rename(rel, dir); err != nil {
		return err
	}
	return s.db.restoreDeletedVersion(id)
}

// PurgeVersion permanently deletes a trashed version (archive + registry row).
// This is the single explicit escape hatch from the trash; built-in versions
// are protected by the engine layer.
func (s *Store) PurgeVersion(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel, ok, err := s.trashVersionDir(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("version %q not found in trash", id)
	}
	if err := os.RemoveAll(rel); err != nil {
		return err
	}
	return s.db.deleteVersion(id)
}

// trashVersionsRoot returns the hidden trash directory inside the data dir.
func (s *Store) trashVersionsRoot() string {
	return filepath.Join(s.dir, ".trash", "versions")
}

// trashVersionDir resolves the relocatable trash directory for a version id
// (created once per trash) and reports whether it exists.
func (s *Store) trashVersionDir(id string) (string, bool, error) {
	root := s.trashVersionsRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	prefix := id + "."
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			return filepath.Join(root, e.Name()), true, nil
		}
	}
	return "", false, nil
}

// sendVersionToTrash relocates a version's archive into the trash with a
// timestamped, collision-free directory name before soft-deleting its row.
func (s *Store) sendVersionToTrash(id string) error {
	dir := filepath.Join(s.verDir, id)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("version %q has no archive", id)
	}
	root := s.trashVersionsRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	name := id + "." + time.Now().Format("20060102-150405")
	rel := filepath.Join(root, name)
	for {
		if _, err := os.Stat(rel); os.IsNotExist(err) {
			break
		}
		name = id + "." + fmt.Sprintf("%d", time.Now().UnixNano())
		rel = filepath.Join(root, name)
	}
	if err := os.Rename(dir, rel); err != nil {
		return err
	}
	return s.db.softDeleteVersion(id)
}

// Checkpoint flushes the WAL into the main database file so a file-level
// backup of the data dir is self-consistent.
func (s *Store) Checkpoint() error {
	return s.db.checkpoint()
}

// ---------------------------------------------------------------------------
// Verse text archive (gzipped JSON, one file per book)
// ---------------------------------------------------------------------------

// bookFileName returns the archive filename for a 1-based book ordinal.
func bookFileName(ordinal int) string {
	return fmt.Sprintf("%04d.json.gz", ordinal)
}

// PutBook stores the full text of one book (all its chapters) for a version and
// collection, replacing any existing text for that book.
func (s *Store) PutBook(id string, collection string, bookOrdinal int, content *model.BookContent) error {
	if id == "" {
		return errors.New("version ID required")
	}
	if collection == "" {
		collection = "bible"
	}
	c, ok := collections.ByID(collection)
	if !ok {
		return fmt.Errorf("unknown collection %q", collection)
	}
	clist := c.Books()
	if bookOrdinal < 1 || bookOrdinal > len(clist) {
		return fmt.Errorf("invalid book ordinal %d in collection %s", bookOrdinal, collection)
	}
	content.VersionID = id
	content.Collection = collection
	content.Book = bookOrdinal
	content.BookName = clist[bookOrdinal-1].Name
	dir := filepath.Join(s.verDir, id, safeColl(collection), "bk")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, bookFileName(bookOrdinal))
	tmp, err := os.CreateTemp(dir, ".bk-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	gz := gzip.NewWriter(tmp)
	enc := json.NewEncoder(gz)
	if err := enc.Encode(content); err != nil {
		gz.Close()
		tmp.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Atomic replace so a crash never leaves a truncated .json.gz.
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDir(dir)
}

// GetBook loads a book's full text for a version and collection, if present.
func (s *Store) GetBook(id string, collection string, bookOrdinal int) (*model.BookContent, bool, error) {
	if collection == "" {
		collection = "bible"
	}
	dir := filepath.Join(s.verDir, id, safeColl(collection), "bk")
	path := filepath.Join(dir, bookFileName(bookOrdinal))
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, false, err
	}
	defer gz.Close()
	var bc model.BookContent
	if err := json.NewDecoder(gz).Decode(&bc); err != nil {
		return nil, false, err
	}
	return &bc, true, nil
}

func safeColl(collection string) string {
	// collection IDs are short slugs; sanitize defensively.
	clean := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, collection)
	if clean == "" {
		return "default"
	}
	return clean
}

// HasBook reports whether a version has text for the given book ordinal in a
// collection.
func (s *Store) HasBook(id string, collection string, bookOrdinal int) bool {
	_, ok, _ := s.GetBook(id, collection, bookOrdinal)
	return ok
}

// VerseText fetches a single verse's text (empty string if missing).
func (s *Store) VerseText(id string, r model.Ref) (string, bool, error) {
	col := r.Collection
	if col == "" {
		col = "bible"
	}
	bc, ok, err := s.GetBook(id, col, r.Book)
	if err != nil {
		return "", false, err
	}
	if !ok || bc.Chapters == nil {
		return "", false, nil
	}
	ch, ok := bc.Chapters[r.Chapter]
	if !ok || ch == nil || ch.Verses == nil {
		return "", false, nil
	}
	if txt, found := ch.Verses[r.Verse]; found {
		return textclean.HTML(txt), true, nil
	}
	return "", false, nil
}

// ListVersionBooks returns the set of book ordinals present in a version
// archive within a collection.
func (s *Store) ListVersionBooks(id string, collection string) ([]int, error) {
	if collection == "" {
		collection = "bible"
	}
	dir := filepath.Join(s.verDir, id, safeColl(collection), "bk")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var out []int
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json.gz") {
			if n, err := strconv.Atoi(strings.TrimSuffix(name, ".json.gz")); err == nil {
				out = append(out, n)
			}
		}
	}
	return out, nil
}

// ImportedBooks returns a map of book ordinal -> true for a version within a
// collection. Used to track import completeness against the full list.
func (s *Store) ImportedBooks(id string, collection string) (map[int]bool, error) {
	ords, err := s.ListVersionBooks(id, collection)
	if err != nil {
		return nil, err
	}
	m := make(map[int]bool, len(ords))
	for _, o := range ords {
		m[o] = true
	}
	return m, nil
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".json-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDir(dir)
}

// syncDir fsyncs a directory so a prior rename is durable on disk.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// GetSetting reads a named preference.
func (s *Store) GetSetting(key string) (string, error) {
	return s.db.getSetting(key)
}

// SetSetting writes a named preference.
func (s *Store) SetSetting(key, value string) error {
	return s.db.setSetting(key, value)
}
