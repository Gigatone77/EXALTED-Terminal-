package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gigatone/biblelearn/internal/model"
	_ "modernc.org/sqlite"
)

// sqlDB wraps the underlying *sql.DB with a mutex, since the pure-Go sqlite
// driver is fine with serialized access and we want a simple, safe layer.
type sqlDB struct {
	*sql.DB
	writeMu sync.Mutex
}

func openDB(path string) (*sqlDB, error) {
	// _pragma enforces foreign keys; busy_timeout avoids SQLITE_BUSY storms.
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Good defaults for a desktop app.
	db.SetMaxOpenConns(1)
	return &sqlDB{DB: db}, nil
}

func migrate(db *sqlDB) error {
	// Repair an older contentless FTS table (USING fts5(..., content='')),
	// where the column values are not stored and full-text search cannot read
	// back the version/verse columns. Reconstruct it as a normal contentful
	// FTS5 table; the verse index is repopulated by Store.Opened()/ReindexAll.
	if err := repairContentlessFTS(db); err != nil {
		return fmt.Errorf("migrate fts: %w", err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS versions (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			lang       TEXT NOT NULL DEFAULT 'en',
			source     TEXT NOT NULL DEFAULT '',
			source_url TEXT NOT NULL DEFAULT '',
			builtin    INTEGER NOT NULL DEFAULT 0,
			enabled    INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);`,
		`CREATE TABLE IF NOT EXISTS notes (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			version_id TEXT NOT NULL,
			book       INTEGER NOT NULL,
			chapter    INTEGER NOT NULL,
			verse      INTEGER NOT NULL,
			body       TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(version_id, book, chapter, verse)
		);`,
		// Spaced-repetition memory state per verse.
		`CREATE TABLE IF NOT EXISTS memory (
			version_id TEXT NOT NULL,
			book       INTEGER NOT NULL,
			chapter    INTEGER NOT NULL,
			verse      INTEGER NOT NULL,
			-- scheduling state
			interval_days REAL NOT NULL DEFAULT 0,
			ease        REAL NOT NULL DEFAULT 2.5,
			reps        INTEGER NOT NULL DEFAULT 0,
			due         TEXT NOT NULL DEFAULT (datetime('now')),
			state       INTEGER NOT NULL DEFAULT 0, -- 0=new 1=learning 2=review
			reviewed_at TEXT,
			PRIMARY KEY(version_id, book, chapter, verse)
		);`,
		// Quiz history.
		`CREATE TABLE IF NOT EXISTS quiz (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			mode       TEXT NOT NULL,
			version_id TEXT NOT NULL,
			book       INTEGER NOT NULL,
			chapter    INTEGER NOT NULL,
			total      INTEGER NOT NULL,
			correct    INTEGER NOT NULL,
			taken_at   TEXT NOT NULL DEFAULT (datetime('now'))
		);`,
		// Full-text search index over currently-enabled versions.
		`CREATE VIRTUAL TABLE IF NOT EXISTS verse_fts USING fts5(
			version, collection, book, chapter, verse, text
		);`,
		// Simple key/value settings (active version, prefs).
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		);`,
		// Term index for the search-box suggestion/autocomplete feature.
		// Maps a normalized term to how often it appears across a version+
		// collection, so common content words surface first.
		`CREATE TABLE IF NOT EXISTS term_index (
			term       TEXT NOT NULL,
			version    TEXT NOT NULL,
			collection TEXT NOT NULL DEFAULT 'bible',
			count      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(term, version, collection)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_term_prefix ON term_index(term, count DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_notes_loc ON notes(version_id, book, chapter, verse);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_due ON memory(due);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// repairContentlessFTS detects a verse_fts virtual table that was created as a
// contentless FTS5 table (content=”), which does not retain the column values
// needed by IndexBook/Search, and rebuilds it contentful. It is idempotent.
func repairContentlessFTS(db *sqlDB) error {
	var ddl string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='verse_fts'`).Scan(&ddl)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // never created; CREATE IF NOT EXISTS below will make it contentful
		}
		return err
	}
	if !strings.Contains(strings.ToLower(ddl), "content=''") {
		return nil // already contentful
	}
	// Drop the broken contentless table and let the normal CREATE and the
	// post-open reindex repopulate it with stored verses.
	if _, err := db.Exec(`DROP TABLE verse_fts`); err != nil {
		return err
	}
	return nil
}

func (db *sqlDB) upsertVersion(v model.Version) error {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()
	_, err := db.Exec(`
		INSERT INTO versions(id, name, lang, source, source_url, builtin, enabled)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			lang=excluded.lang,
			source=excluded.source,
			source_url=excluded.source_url,
			builtin=excluded.builtin,
			enabled=excluded.enabled`,
		v.ID, v.Name, v.Lang, v.Source, v.SourceURL, b2i(v.Builtin), b2i(v.Enabled))
	return err
}

func (db *sqlDB) listVersions() ([]model.Version, error) {
	rows, err := db.Query(`SELECT id, name, lang, source, source_url, builtin, enabled FROM versions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Version
	for rows.Next() {
		var v model.Version
		var bi, en int
		if err := rows.Scan(&v.ID, &v.Name, &v.Lang, &v.Source, &v.SourceURL, &bi, &en); err != nil {
			return nil, err
		}
		v.Builtin = bi > 0
		v.Enabled = en > 0
		out = append(out, v)
	}
	return out, rows.Err()
}

func (db *sqlDB) getVersion(id string) (model.Version, bool, error) {
	row := db.QueryRow(`SELECT id, name, lang, source, source_url, builtin, enabled FROM versions WHERE id=?`, id)
	var v model.Version
	var bi, en int
	err := row.Scan(&v.ID, &v.Name, &v.Lang, &v.Source, &v.SourceURL, &bi, &en)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Version{}, false, nil
	}
	if err != nil {
		return model.Version{}, false, err
	}
	v.Builtin = bi > 0
	v.Enabled = en > 0
	return v, true, nil
}

func (db *sqlDB) deleteVersion(id string) error {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()
	_, err := db.Exec(`DELETE FROM versions WHERE id=?`, id)
	return err
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// GetSetting reads a settings value ("" if unset).
func (db *sqlDB) getSetting(key string) (string, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err != nil && err.Error() == "sql: no rows in result set" {
		return "", nil
	}
	return v, err
}

// SetSetting writes a settings value.
func (db *sqlDB) setSetting(key, value string) error {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()
	_, err := db.Exec(`INSERT INTO settings(key, value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
