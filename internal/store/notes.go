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

// GetNote returns the active (non-trashed) note for a verse, if any.
func (s *Store) GetNote(versionID string, book, chapter, verse int) (Note, bool, error) {
	row := s.db.QueryRow(`SELECT version_id, book, chapter, verse, body, updated_at
		FROM notes WHERE version_id=? AND book=? AND chapter=? AND verse=? AND deleted_at IS NULL`,
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

// ListNotes returns all active notes for a version.
func (s *Store) ListNotes(versionID string) ([]Note, error) {
	rows, err := s.db.Query(`SELECT version_id, book, chapter, verse, body, updated_at
		FROM notes WHERE version_id=? AND deleted_at IS NULL ORDER BY book, chapter, verse`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotes(rows)
}

// ListTrashedNotes returns notes currently in the recoverable trash.
func (s *Store) ListTrashedNotes(versionID string) ([]Note, error) {
	rows, err := s.db.Query(`SELECT version_id, book, chapter, verse, body, updated_at
		FROM notes WHERE version_id=? AND deleted_at IS NOT NULL ORDER BY book, chapter, verse`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotes(rows)
}

// PurgeTrashedNotes permanently removes trashed notes for a version ("" = all
// versions). This is the explicit escape hatch from the trash.
func (s *Store) PurgeTrashedNotes(versionID string) (int64, error) {
	s.db.writeMu.Lock()
	defer s.db.writeMu.Unlock()
	if versionID != "" {
		res, err := s.db.Exec(`DELETE FROM notes WHERE deleted_at IS NOT NULL AND version_id=?`, versionID)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	res, err := s.db.Exec(`DELETE FROM notes WHERE deleted_at IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanNotes(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]Note, error) {
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