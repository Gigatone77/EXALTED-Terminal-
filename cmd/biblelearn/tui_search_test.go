package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/gigatone/biblelearn/internal/engine"
)

func TestTUISeedIndexes(t *testing.T) {
	e, err := engine.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err := e.SeedEmbeddedKJV(); err != nil {
		t.Fatal(err)
	}
	m, err := newAppModel(e)
	if err != nil {
		t.Fatal(err)
	}
	m.searchInput.SetValue("love")
	m.runSearch()
	t.Logf("search results: %d", len(m.results))
	t.Logf("m.activeVersion = %q", m.activeVersion)
	aid, _ := e.ActiveVersionID()
	t.Logf("ActiveVersionID = %q", aid)

	db, err := sql.Open("sqlite", "file:"+filepath.Join(e.Store.Dir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var ver string
	_ = db.QueryRow("SELECT DISTINCT version FROM verse_fts LIMIT 1").Scan(&ver)
	t.Logf("stored verse_fts version = %q", ver)
	resAll, _ := e.Store.Search("love", "", "", 5)
	resKJV, _ := e.Store.Search("love", "KJV", "", 5)
	t.Logf("Search(all)=%d Search(KJV)=%d", len(resAll), len(resKJV))
	var matched int
	err = db.QueryRow(`SELECT count(*) FROM verse_fts WHERE verse_fts MATCH '"love"'`).Scan(&matched)
	t.Logf("direct MATCH=\"love\" -> matched=%d err=%v", matched, err)
	if len(m.results) == 0 {
		t.Fatalf("no TUI results (all=%d kjv=%d direct=%d)", len(resAll), len(resKJV), matched)
	}
}
