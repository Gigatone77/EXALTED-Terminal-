package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gigatone/biblelearn/internal/books"
	"github.com/gigatone/biblelearn/internal/collections"
)

// SearchResult is one full-text search hit.
type SearchResult struct {
	VersionID  string
	Collection string
	Book       int
	Chapter    int
	Verse      int
	Text       string
	BookName   string
	Score      float64
	// BookSize is the canonical verse count of the containing book, used to
	// prioritize results from smaller books (e.g. Amos, Habakkuk).
	BookSize int
}

// InsertBooksIndex adds/updates every verse of a book in the full-text index
// for a version+collection. It should be called whenever a book is written or
// replaced.
func (s *Store) IndexBook(versionID, collection string, bookOrdinal int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bc, ok, err := s.GetBook(versionID, collection, bookOrdinal)
	if err != nil {
		return err
	}
	s.db.writeMu.Lock()
	defer s.db.writeMu.Unlock()
	if !ok {
		_, err := s.db.Exec(`DELETE FROM verse_fts WHERE version=? AND collection=? AND book=?`,
			versionID, collection, bookOrdinal)
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM verse_fts WHERE version=? AND collection=? AND book=?`,
		versionID, collection, bookOrdinal); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO verse_fts(version, collection, book, chapter, verse, text) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, cn := range bc.ChapterOrder {
		ch, exists := bc.Chapters[cn]
		if !exists || ch == nil {
			continue
		}
		for _, vn := range ch.Order {
			text, ok := ch.Verses[vn]
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			if _, err := stmt.Exec(versionID, collection, bookOrdinal, cn, vn, text); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// Search runs a full-text query across the given (version, collection) pairs
// (empty means all indexed) and returns up to limit hits ordered by relevance.
func (s *Store) Search(query string, version, collection string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}
	q := ftsQuery(query)
	if q == "" {
		return nil, nil
	}
	args := []any{q}
	where := ""
	if version != "" {
		where += " AND version=?"
		args = append(args, version)
	}
	if collection != "" {
		where += " AND collection=?"
		args = append(args, collection)
	}
	// Fetch a generous candidate pool so that re-ranking by book size does not
	// discard hits that only appear in small books beyond the initial top-N.
	fetchN := limit * 3
	if fetchN < 200 {
		fetchN = 200
	}
	args = append(args, fetchN)
	sql := `SELECT version, collection, book, chapter, verse, text, bm25(verse_fts) AS score
	        FROM verse_fts
	        WHERE verse_fts MATCH ?` + where + `
	        ORDER BY score LIMIT ?`
	rows, err := s.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.VersionID, &r.Collection, &r.Book, &r.Chapter, &r.Verse, &r.Text, &r.Score); err != nil {
			return nil, err
		}
		r.BookSize = books.VerseCount(r.Book)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Prioritize results from smaller books (fewer verses), then by relevance.
	// Lower bm25 score means higher relevance.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].BookSize != out[j].BookSize {
			return out[i].BookSize < out[j].BookSize
		}
		return out[i].Score < out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ftsQuery converts user input into an FTS5 MATCH expression. It quotes each
// token so user punctuation doesn't break the query.
func ftsQuery(q string) string {
	fields := strings.Fields(q)
	if len(fields) == 0 {
		return ""
	}
	var parts []string
	for _, f := range fields {
		f = strings.Trim(f, `"(),`)
		if f == "" {
			continue
		}
		parts = append(parts, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " ")
}

// ReindexAllBooks repopulates the full-text index for every verse of every book
// across all installed versions and collections. It is used to rebuild a broken
// or empty index (e.g. after repairing a contentless FTS table).
func (s *Store) ReindexAllBooks() error {
	versions, err := s.db.listVersions()
	if err != nil {
		return err
	}
	for _, v := range versions {
		for _, c := range collections.All() {
			for _, b := range c.Books() {
				path := filepath.Join(s.verDir, v.ID, safeColl(c.ID), "bk", bookFileName(b.Ordinal))
				if _, err := os.Stat(path); err != nil {
					continue // book not present in this version/collection
				}
				if err := s.IndexBook(v.ID, c.ID, b.Ordinal); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
