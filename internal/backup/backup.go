// Package backup provides a crash- and flash-safe snapshot of the app's data
// directory. It mirrors the durability discipline used by the Gigatone flash
// tools: stage into a temp dir, fsync every file and directory, then move the
// named backup into place with an atomic rename so a power loss can never
// leave a half-written backup behind.
package backup

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// checkpointBackend is the callback the caller uses to flush the SQLite WAL as
// closed-checkpoint values ready for the backup. It may be omitted when data
// lives only in regular files.
type checkpointBackend func() error

// CopyDir copies the contents of src into dst (which need not exist), skipping
// any relative path for which skip returns true (destRoot subtrees nested inside
// src are excluded so a backup never backs itself up). It fsyncs each created
// file and directory, making the copy durable before it is "published" by the
// caller's rename.
func CopyDir(src, dst string, skip func(rel string) bool) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if skip != nil && skip(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		out.Close()
		os.Remove(dst)
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		return fail(err)
	}
	if err := out.Sync(); err != nil {
		return fail(err)
	}
	if err := out.Close(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

// Snapshot creates `exalted-backup-YYYYMMDD-HHMMSS` next to (inside) destRoot.
// It checkpoints the store, stages a full copy of dataDir into a hidden
// `.<name>.tmp` directory, fsyncs it, and atomically renames it into place.
// Returns the final backup path.
func Snapshot(dataDir, destRoot string, checkpoint checkpointBackend) (string, error) {
	base := "exalted-backup-" + time.Now().Format("20060102-150405")
	name := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(destRoot, name)); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	dest := filepath.Join(destRoot, name)
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return "", err
	}
	staging := filepath.Join(destRoot, "."+name+".tmp")
	// Remove a stale staging dir left by a previous crashed run.
	if err := os.RemoveAll(staging); err != nil {
		return "", err
	}
	if checkpoint != nil {
		if err := checkpoint(); err != nil {
			return "", fmt.Errorf("checkpoint store: %w", err)
		}
	}
	// When the destination lives inside the data dir, exclude that whole
	// subtree from the walk (the destRoot top-level dir) — otherwise the copy
	// would re-ingest staging/older backups over and over.
	destRootBase := filepath.Base(destRoot)
	skip := func(rel string) bool {
		first := rel
		if i := strings.IndexByte(rel, os.PathSeparator); i >= 0 {
			first = rel[:i]
		}
		return first == destRootBase
	}
	if err := CopyDir(dataDir, staging, skip); err != nil {
		os.RemoveAll(staging)
		return "", err
	}
	if err := syncTree(staging); err != nil {
		os.RemoveAll(staging)
		return "", err
	}
	if err := syncDir(staging); err != nil {
		os.RemoveAll(staging)
		return "", err
	}
	if err := os.Rename(staging, dest); err != nil {
		os.RemoveAll(staging)
		return "", err
	}
	if err := syncDir(destRoot); err != nil {
		return "", err
	}
	return dest, nil
}

// syncTree recursively fsyncs every file under root. Snapshotted backups should
// not be used until both the files and the directory containing them are
// durable, so callers fsync the produced backup too.
func syncTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return syncDir(path)
	})
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Newest returns the newest backup directory (if any) under destRoot, useful
// for a `backup list` style listing.
func Newest(destRoot string) (string, error) {
	entries, err := os.ReadDir(destRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	best := ""
	bestT := time.Time{}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "exalted-backup-") || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(bestT) {
			best = filepath.Join(destRoot, e.Name())
			bestT = info.ModTime()
		}
	}
	return best, nil
}