package store

import (
	"github.com/gigatone/biblelearn/internal/textclean"
	"github.com/gigatone/biblelearn/internal/words"
)

// TermSuggestion is a term suggested for the search box.
type TermSuggestion struct {
	Term  string
	Count int
}

// RebuildTermIndex recounts every term across all stored books of a version+
// collection, replacing the previous term index for it. It should be called
// after installing or materially changing a version.
func (s *Store) RebuildTermIndex(versionID, collection string) error {
	if collection == "" {
		collection = "bible"
	}
	counts := map[string]int{}
	books, err := s.ListVersionBooks(versionID, collection)
	if err != nil {
		return err
	}
	for _, ord := range books {
		bc, ok, err := s.GetBook(versionID, collection, ord)
		if err != nil || !ok {
			continue
		}
		for _, cn := range bc.ChapterOrder {
			ch := bc.Chapters[cn]
			if ch == nil {
				continue
			}
			for _, vn := range ch.Order {
				text := textclean.HTML(ch.Verses[vn])
				for _, tok := range words.Tokenize(text) {
					if words.IsCommon(tok.Term) {
						continue
					}
					counts[tok.Term]++
				}
			}
		}
	}
	s.db.writeMu.Lock()
	defer s.db.writeMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM term_index WHERE version=? AND collection=?`, versionID, collection); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO term_index(term, version, collection, count) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for term, c := range counts {
		if _, err := stmt.Exec(term, versionID, collection, c); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SuggestTerms returns up to limit terms that start with prefix, ranked by
// frequency (common words first). If freq is false, terms are returned
// alphabetically instead. version may be empty to aggregate across versions.
func (s *Store) SuggestTerms(prefix string, version, collection string, limit int, freq bool) ([]TermSuggestion, error) {
	if limit <= 0 {
		limit = 30
	}
	if collection == "" {
		collection = "bible"
	}
	// Range scan over the term primary-key prefix.
	end := prefix + "\uffff"
	args := []any{prefix, end, limit * 8}
	sql := `SELECT term, SUM(count) AS c FROM term_index
	        WHERE term >= ? AND term < ?`
	if version != "" {
		sql += ` AND version=?`
		args = append(args, version)
	}
	if collection != "" {
		sql += ` AND collection=?`
		args = append(args, collection)
	}
	sql += ` GROUP BY term `
	if freq {
		sql += `ORDER BY c DESC `
	} else {
		sql += `ORDER BY term `
	}
	sql += `LIMIT ?`
	rows, err := s.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TermSuggestion
	for rows.Next() {
		var t TermSuggestion
		if err := rows.Scan(&t.Term, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, rows.Err()
}

// TermCount returns how many times a term appears in a version+collection.
func (s *Store) TermCount(term, version, collection string) (int, error) {
	if collection == "" {
		collection = "bible"
	}
	var c int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM term_index WHERE term=? AND version=? AND collection=?`,
		term, version, collection).Scan(&c)
	if err != nil {
		return 0, err
	}
	return c, nil
}

// TermIndexedVersions returns which versions have a populated term index for a
// collection. Useful to know what's searchable.
func (s *Store) TermIndexedVersions(collection string) ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT version FROM term_index WHERE collection=?`, collection)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
