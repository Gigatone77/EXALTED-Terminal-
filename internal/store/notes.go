package store

// Note is a user's study note attached to a verse.
type Note struct {
	VersionID string
	Book      int
	Chapter   int
	Verse     int
	Body      string
	UpdatedAt string
}

// SaveNote upserts the note body for a verse (empty body deletes it).
func (s *Store) SaveNote(n Note) error {
	s.db.writeMu.Lock()
	defer s.db.writeMu.Unlock()
	if n.Body == "" {
		_, err := s.db.Exec(`DELETE FROM notes WHERE version_id=? AND book=? AND chapter=? AND verse=?`,
			n.VersionID, n.Book, n.Chapter, n.Verse)
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO notes(version_id, book, chapter, verse, body, updated_at)
		VALUES(?,?,?,?,?,datetime('now'))
		ON CONFLICT(version_id, book, chapter, verse) DO UPDATE SET
			body=excluded.body, updated_at=datetime('now')`,
		n.VersionID, n.Book, n.Chapter, n.Verse, n.Body)
	return err
}

// GetNote returns the note for a verse, if any.
func (s *Store) GetNote(versionID string, book, chapter, verse int) (Note, bool, error) {
	row := s.db.QueryRow(`SELECT version_id, book, chapter, verse, body, updated_at
		FROM notes WHERE version_id=? AND book=? AND chapter=? AND verse=?`,
		versionID, book, chapter, verse)
	var n Note
	err := row.Scan(&n.VersionID, &n.Book, &n.Chapter, &n.Verse, &n.Body, &n.UpdatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return Note{}, false, nil
		}
		return Note{}, false, err
	}
	return n, true, nil
}

// ListNotes returns all notes for a version.
func (s *Store) ListNotes(versionID string) ([]Note, error) {
	rows, err := s.db.Query(`SELECT version_id, book, chapter, verse, body, updated_at
		FROM notes WHERE version_id=? ORDER BY book, chapter, verse`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.VersionID, &n.Book, &n.Chapter, &n.Verse, &n.Body, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
