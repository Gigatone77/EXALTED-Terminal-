package store

import (
	"context"
	"database/sql"
	"time"
)

// GetCache returns a cached value for a key, if present.
func (s *Store) GetCache(ctx context.Context, key string) (string, bool, error) {
	if err := s.ensureCacheTable(); err != nil {
		return "", false, err
	}
	var val string
	var expires int64
	err := s.db.QueryRowContext(ctx,
		`SELECT value, expire_ts FROM cache WHERE key=?`, key).Scan(&val, &expires)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if expires > 0 && expires < time.Now().Unix() {
		_, _ = s.db.Exec(`DELETE FROM cache WHERE key=?`, key)
		return "", false, nil
	}
	return val, true, nil
}

// PutCache stores a value for a key, expiring after ttl (0 = never).
func (s *Store) PutCache(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := s.ensureCacheTable(); err != nil {
		return err
	}
	var expires int64
	if ttl > 0 {
		expires = time.Now().Add(ttl).Unix()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cache(key, value, expire_ts) VALUES(?,?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, expire_ts=excluded.expire_ts`,
		key, value, expires)
	return err
}

func (s *Store) ensureCacheTable() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS cache(
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		expire_ts INTEGER DEFAULT 0)`)
	return err
}
