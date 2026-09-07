package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gigatone/biblelearn/internal/model"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestNoteSoftDeleteRestore(t *testing.T) {
	s := openTestStore(t)
	loc := Note{VersionID: "KJV", Book: 1, Chapter: 3, Verse: 16}
	if err := s.SaveNote(Note{VersionID: loc.VersionID, Book: loc.Book, Chapter: loc.Chapter, Verse: loc.Verse, Body: "seed note"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, ok, _ := s.GetNote(loc.VersionID, loc.Book, loc.Chapter, loc.Verse); !ok {
		t.Fatal("note should be visible before delete")
	}
	if err := s.SaveNote(Note{VersionID: loc.VersionID, Book: loc.Book, Chapter: loc.Chapter, Verse: loc.Verse}); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}
	if _, ok, _ := s.GetNote(loc.VersionID, loc.Book, loc.Chapter, loc.Verse); ok {
		t.Fatal("note should be hidden after delete")
	}
	if ns, _ := s.ListTrashedNotes(loc.VersionID); len(ns) != 1 {
		t.Fatalf("trash should hold 1 note, got %d", len(ns))
	}
	if restored, err := s.RestoreNote(loc.VersionID, loc.Book, loc.Chapter, loc.Verse); err != nil || !restored {
		t.Fatalf("restore: restored=%v err=%v", restored, err)
	}
	// Re-writing a body to the location also brings it back (undo semantics).
	if err := s.SaveNote(Note{VersionID: loc.VersionID, Book: loc.Book, Chapter: loc.Chapter, Verse: loc.Verse, Body: "new body"}); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	n, ok, _ := s.GetNote(loc.VersionID, loc.Book, loc.Chapter, loc.Verse)
	if !ok || n.Body != "new body" {
		t.Fatalf("expected restored note with new body, got %+v ok=%v", n, ok)
	}
}

func TestNotePurge(t *testing.T) {
	s := openTestStore(t)
	for i := 1; i <= 3; i++ {
		if err := s.SaveNote(Note{VersionID: "KJV", Book: 1, Chapter: 1, Verse: i, Body: "note"}); err != nil {
			t.Fatalf("save: %v", err)
		}
		if err := s.SaveNote(Note{VersionID: "KJV", Book: 1, Chapter: 1, Verse: i}); err != nil {
			t.Fatalf("delete: %v", err)
		}
	}
	n, err := s.PurgeTrashedNotes("KJV")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 purged, got %d", n)
	}
	if ns, _ := s.ListTrashedNotes("KJV"); len(ns) != 0 {
		t.Fatalf("trash should be empty, got %d", len(ns))
	}
}

func TestVersionTrashRestorePurge(t *testing.T) {
	s := openTestStore(t)
	v := model.Version{ID: "MINI", Name: "Mini", Lang: "en", Source: "test"}
	if _, err := writeTestBook(s, v.ID); err != nil {
		t.Fatalf("write book: %v", err)
	}
	if err := s.RegisterVersion(v); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok, _ := s.GetVersion(v.ID); !ok {
		t.Fatal("version should be present before rm")
	}
	if err := s.RemoveVersion(v.ID); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if _, ok, _ := s.GetVersion(v.ID); ok {
		t.Fatal("version should be hidden after rm")
	}
	vs, _ := s.TrashedVersions()
	if len(vs) != 1 {
		t.Fatalf("trash should hold 1 version, got %d", len(vs))
	}
	// Archive dir must physically sit in the trash, not be deleted.
	entries, _ := os.ReadDir(filepath.Join(s.Dir(), ".trash", "versions"))
	found := false
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) > len(vs[0].ID)+1 && e.Name()[:len(vs[0].ID)] == vs[0].ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a trashed archive dir under .trash/versions, entries=%v", entries)
	}
	if err := s.RestoreVersion(v.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, ok, _ := s.GetVersion(v.ID); !ok {
		t.Fatal("version should be back after restore")
	}
	// Removing again + purge permanently deletes.
	if err := s.RemoveVersion(v.ID); err != nil {
		t.Fatalf("rm2: %v", err)
	}
	if err := s.PurgeVersion(v.ID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if vs, _ := s.TrashedVersions(); len(vs) != 0 {
		t.Fatalf("trash should be empty after purge, got %d", len(vs))
	}
	if err := s.RestoreVersion(v.ID); err == nil {
		t.Fatal("restore after purge should fail")
	}
}

func writeTestBook(s *Store, id string) (int, error) {
	bc := model.NewBookContent(id, "bible", 1)
	ch := &model.Chapter{VersionID: id, Collection: "bible", Book: 1, Number: 1, Verses: map[int]string{1: "hello"}, Order: []int{1}}
	bc.Chapters[1] = ch
	bc.ChapterOrder = []int{1}
	return 1, s.PutBook(id, "bible", 1, bc)
}
