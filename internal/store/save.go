package store

// SaveNote upserts the note body for a verse. An empty body soft-deletes the
// active note (moves it to the recoverable trash) instead of destroying it;
// writing a body to a trashed location brings it back (undo semantics).
func (s *Store) SaveNote(n Note) error {
	s.db.writeMu.Lock()
	defer s.db.writeMu.Unlock()
	if n.Body == "" {
		_, err := s.db.Exec(`UPDATE notes SET deleted_at=datetime('now')
			WHERE version_id=? AND book=? AND chapter=? AND verse=? AND deleted_at IS NULL`,
			n.VersionID, n.Book, n.Chapter, n.Verse)
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO notes(version_id, book, chapter, verse, body, updated_at)
		VALUES(?,?,?,?,?,datetime('now'))
		ON CONFLICT(version_id, book, chapter, verse) DO UPDATE SET
			body=excluded.body, updated_at=datetime('now'), deleted_at=NULL`,
		n.VersionID, n.Book, n.Chapter, n.Verse, n.Body)
	return err
}

// RestoreNote brings a trashed note back for the given verse. It reports
// whether a note was actually recovered.
func (s *Store) RestoreNote(versionID string, book, chapter, verse int) (bool, error) {
	s.db.writeMu.Lock()
	defer s.db.writeMu.Unlock()
	res, err := s.db.Exec(`UPDATE notes SET deleted_at=NULL
		WHERE version_id=? AND book=? AND chapter=? AND verse=? AND deleted_at IS NOT NULL`,
		versionID, book, chapter, verse)
	if err != nil {
		return false, err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		return true, nil
	}
	return false, nil
}
