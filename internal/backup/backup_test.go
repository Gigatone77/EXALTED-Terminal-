package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotNestedDest(t *testing.T) {
	src := t.TempDir()
	// populate a fake store: one DB file + a version archive
	dbPath := filepath.Join(src, "app.db")
	if err := os.WriteFile(dbPath, []byte("somedb"), 0o644); err != nil {
		t.Fatal(err)
	}
	verDir := filepath.Join(src, "versions", "KJV", "bk")
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(verDir, "0001.json.gz"), []byte("book"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The tricky case: destination INSIDE the source dir (dataDir/backs).
	destRoot := filepath.Join(src, "backs")
	out, err := Snapshot(src, destRoot, nil)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// Sanity: no runaway nesting.
	if strings.Contains(out, "backs/backs/") {
		t.Fatalf("backup recursively copied itself into %q", out)
	}
	// The backup must contain the db and version files but not the backups dir.
	for _, want := range []string{"app.db", filepath.Join("versions", "KJV", "bk", "0001.json.gz")} {
		if _, err := os.Stat(filepath.Join(out, want)); err != nil {
			t.Errorf("backup missing %s: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "backs")); err == nil {
		t.Error("backup should not contain its own destination dir")
	}
	// A second snapshot must be independent (timestamped) and not stomp the first.
	out2, err := Snapshot(src, destRoot, nil)
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if out2 == out {
		t.Fatal("second snapshot reused the same directory")
	}
}

func TestSnapshotOutsideDest(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "app.db"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	destRoot := filepath.Join(t.TempDir(), "backups")
	out, err := Snapshot(src, destRoot, nil)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "app.db")); err != nil {
		t.Fatalf("backup missing app.db: %v", err)
	}
}
