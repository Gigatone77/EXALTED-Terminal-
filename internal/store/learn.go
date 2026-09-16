package store

import (
	"time"
)

// MemoryState describes the spaced-repetition state of a verse in a version.
type MemoryState struct {
	VersionID    string
	Book         int
	Chapter      int
	Verse        int
	IntervalDays float64
	Ease         float64
	Reps         int
	State        int // 0=new 1=learning 2=review
	Due          time.Time
}

// SaveMemory updates/inserts the memory state for a verse.
func (s *Store) SaveMemory(m MemoryState) error {
	s.db.writeMu.Lock()
	defer s.db.writeMu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO memory(version_id, book, chapter, verse, interval_days, ease, reps, due, state)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(version_id, book, chapter, verse) DO UPDATE SET
			interval_days=excluded.interval_days,
			ease=excluded.ease,
			reps=excluded.reps,
			due=excluded.due,
			state=excluded.state,
			reviewed_at=datetime('now')`,
		m.VersionID, m.Book, m.Chapter, m.Verse,
		m.IntervalDays, m.Ease, m.Reps, m.Due.Format(time.RFC3339), m.State)
	return err
}

// GetMemory returns the spaced-repetition state for one verse, if any.
func (s *Store) GetMemory(versionID string, book, chapter, verse int) (MemoryState, bool, error) {
	var m MemoryState
	var dueStr string
	err := s.db.QueryRow(`SELECT version_id, book, chapter, verse, interval_days, ease, reps, due, state
		FROM memory WHERE version_id=? AND book=? AND chapter=? AND verse=?`,
		versionID, book, chapter, verse).Scan(&m.VersionID, &m.Book, &m.Chapter, &m.Verse,
		&m.IntervalDays, &m.Ease, &m.Reps, &dueStr, &m.State)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return MemoryState{}, false, nil
		}
		return MemoryState{}, false, err
	}
	if t, err := time.Parse(time.RFC3339, dueStr); err == nil {
		m.Due = t
	}
	return m, true, nil
}

// DueCards returns memory cards that are due for review for a version.
func (s *Store) DueCards(versionID string, limit int) ([]MemoryState, error) {
	if limit <= 0 {
		limit = 50
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.db.Query(`
		SELECT version_id, book, chapter, verse, interval_days, ease, reps, due, state
		FROM memory
		WHERE version_id=? AND due <= ?
		ORDER BY due ASC LIMIT ?`, versionID, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemory(rows)
}

// Stats returns aggregate memory statistics for a version.
func (s *Store) MemoryStats(versionID string) (total int, due int, learning int, review int, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*) FROM memory WHERE version_id=?`, versionID).Scan(&total)
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	err = s.db.QueryRow(`SELECT COUNT(*) FROM memory WHERE version_id=? AND due<=?`, versionID, now).Scan(&due)
	if err != nil {
		return
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM memory WHERE version_id=? AND state=1`, versionID).Scan(&learning)
	if err != nil {
		return
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM memory WHERE version_id=? AND state=2`, versionID).Scan(&review)
	return
}

// ScheduleNext computes the next review state using SM-2 style scheduling.
func ScheduleNext(m MemoryState, quality int) MemoryState {
	// quality: 0=again 3=hard 4=good 5=easy
	q := float64(quality)
	switch m.State {
	case 0: // new
		m.Reps = 0
		m.IntervalDays = 0
		m.State = 1
		m.Due = time.Now().Add(10 * time.Minute)
	case 1: // learning
		switch quality {
		case 0:
			m.Ease = clamp(m.Ease-0.15, 1.3, 3.5)
			m.IntervalDays = 0
			m.Due = time.Now().Add(10 * time.Minute)
		default:
			m.Reps++
			m.IntervalDays = 1
			m.State = 2
			m.Due = time.Now().Add(24 * time.Hour)
		}
	case 2: // review
		if quality == 0 {
			m.Ease = clamp(m.Ease-0.20, 1.3, 3.5)
			m.IntervalDays = 0
			m.State = 1
			m.Due = time.Now().Add(10 * time.Minute)
			return m
		}
		m.Ease = clamp(m.Ease+(0.1-(5-q)*(0.08+(5-q)*0.02)), 1.3, 3.5)
		if m.Reps == 0 {
			m.IntervalDays = 1
		} else if m.Reps == 1 {
			m.IntervalDays = 6
		} else {
			if quality >= 4 {
				m.IntervalDays = m.IntervalDays * m.Ease
			} else {
				m.IntervalDays = m.IntervalDays * 1.2
			}
		}
		m.Reps++
		m.Due = time.Now().Add(time.Duration(m.IntervalDays * 24 * float64(time.Hour)))
	}
	return m
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func scanMemory(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]MemoryState, error) {
	var out []MemoryState
	for rows.Next() {
		var m MemoryState
		var dueStr string
		if err := rows.Scan(&m.VersionID, &m.Book, &m.Chapter, &m.Verse,
			&m.IntervalDays, &m.Ease, &m.Reps, &dueStr, &m.State); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, dueStr); err == nil {
			m.Due = t
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
