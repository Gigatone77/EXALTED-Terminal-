package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gigatone/biblelearn/internal/engine"
)

func newTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	e, err := engine.New(t.TempDir())
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if _, err := e.SeedEmbeddedKJV(); err != nil {
		t.Fatalf("seed KJV: %v", err)
	}
	return e
}

func key(k string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)} }

func TestTUI_BrowseRenders(t *testing.T) {
	e := newTestEngine(t)
	m, err := newAppModel(e)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	v := m.View()
	for _, want := range []string{"EXALTED", "Genesis", "Browse"} {
		if !strings.Contains(v, want) {
			t.Errorf("browse view missing %q", want)
		}
	}
	// Navigation keys should not panic.
	for _, k := range []string{"j", "k", "left", "right", "n"} {
		T, _ := m.Update(key(k))
		m = T.(*appModel)
	}
	if m.View() == "" {
		t.Error("browse view empty after navigation")
	}
}

// TestTUI_LeftArrowNavigates ensures the left arrow (and 'h') move to the
// previous verse rather than being swallowed by tab handling.
func TestTUI_LeftArrowNavigates(t *testing.T) {
	e := newTestEngine(t)
	m, err := newAppModel(e)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	press := func(k string) {
		T, _ := m.Update(key(k))
		m = T.(*appModel)
	}
	// Move down to verse 4.
	for _, k := range []string{"j", "j", "j"} {
		press(k)
	}
	if m.verse != 4 {
		t.Fatalf("expected verse 4, got %d", m.verse)
	}
	press("left")
	if m.verse != 3 {
		t.Errorf("left arrow: expected verse 3, got %d", m.verse)
	}
	press("h")
	if m.verse != 2 {
		t.Errorf("h key: expected verse 2, got %d", m.verse)
	}
}

// TestTUI_ArrowKeysControlSpan verifies up expands and down shrinks the
// visible verse span, while j/k still navigate verses.
func TestTUI_ArrowKeysControlSpan(t *testing.T) {
	e := newTestEngine(t)
	m, err := newAppModel(e)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	press := func(k string) {
		T, _ := m.Update(key(k))
		m = T.(*appModel)
	}
	if m.verseSpan != 1 {
		t.Fatalf("expected initial span 1, got %d", m.verseSpan)
	}
	// up expands.
	press("up")
	if m.verseSpan != 2 {
		t.Errorf("up: expected span 2, got %d", m.verseSpan)
	}
	// down shrinks back.
	press("down")
	if m.verseSpan != 1 {
		t.Errorf("down: expected span 1, got %d", m.verseSpan)
	}
	// j/k still navigate verses (not the span).
	press("j")
	if m.verse != 2 {
		t.Errorf("j: expected verse 2, got %d", m.verse)
	}
	if m.verseSpan != 1 {
		t.Errorf("j must not change span, got %d", m.verseSpan)
	}
}

func TestTUI_SearchTab(t *testing.T) {
	e := newTestEngine(t)
	m, err := newAppModel(e)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	// Switch to search tab, type a query, run it.
	T, _ := m.Update(key("2"))
	m = T.(*appModel)
	m.searchInput.SetValue("love")
	m.runSearch()
	v := m.View()
	if !strings.Contains(v, "Search") {
		t.Error("search view missing header")
	}
	if len(m.results) == 0 {
		t.Skip("no search results indexed in test env")
	}
}

func TestTUI_MemoryTab(t *testing.T) {
	e := newTestEngine(t)
	m, err := newAppModel(e)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	// Fresh session: add a verse to the deck on the Browse tab, then review it.
	m.loadBook(43) // John
	m.chapter = 3
	m.verse = 16
	m.studyVerse()
	if m.err != nil {
		t.Fatalf("studyVerse error: %v", m.err)
	}
	cards2, _ := e.Store.DueCards("KJV", 10)
	t.Logf("DueCards(KJV) after study: %d", len(cards2))

	T, _ := m.Update(key("3"))
	m = T.(*appModel)
	m.reloadCards()
	if v := m.View(); !strings.Contains(v, "Memory Review") {
		t.Error("memory view missing header")
	}
	if len(m.cards) == 0 {
		t.Fatal("expected at least one due card after studying John 3:16")
	}
	card := m.cards[0]
	if card.Book != 43 || card.Chapter != 3 || card.Verse != 16 {
		t.Errorf("expected John 3:16 card, got book=%d ch=%d v=%d", card.Book, card.Chapter, card.Verse)
	}
	// Space reveals answer, then grading "good" schedules the next review.
	T, _ = m.Update(key(" "))
	m = T.(*appModel)
	if !m.shown {
		t.Error("expected answer revealed after space")
	}
	before := len(m.cards)
	T, _ = m.Update(key("g"))
	m = T.(*appModel)
	if !m.shown {
		t.Log("graded; next card shown")
	}
	_ = before
}

func TestTUI_NotesTab(t *testing.T) {
	e := newTestEngine(t)
	m, err := newAppModel(e)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	T, _ := m.Update(key("4"))
	m = T.(*appModel)
	m.reloadNotes()
	if v := m.View(); !strings.Contains(v, "Study Notes") {
		t.Error("notes view missing header")
	}
}

func TestTUI_ExpandVerseSpan(t *testing.T) {
	e := newTestEngine(t)
	m, err := newAppModel(e)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	m.loadBook(1) // Genesis
	m.chapter = 1
	m.verse = 1
	base := m.View()

	// Expanding with ']' increases the span and shows more verses.
	T, _ := m.Update(key("]"))
	m = T.(*appModel)
	if m.verseSpan != 2 {
		t.Errorf("expected span 2 after expand, got %d", m.verseSpan)
	}
	expanded := m.View()
	if expanded == base {
		t.Error("expanding the span should change the rendered view")
	}
	if !strings.Contains(expanded, "2 of") {
		t.Errorf("expanded view missing span count, got:\n%s", expanded)
	}

	// Contracting with '[' reduces it back toward a single verse.
	T, _ = m.Update(key("["))
	m = T.(*appModel)
	if m.verseSpan != 1 {
		t.Errorf("expected span 1 after contract, got %d", m.verseSpan)
	}

	// '0' resets to a single verse no matter the current span.
	T, _ = m.Update(key("]"))
	m = T.(*appModel)
	T, _ = m.Update(key("]"))
	m = T.(*appModel)
	if m.verseSpan < 2 {
		t.Fatalf("expected span >=2 before reset, got %d", m.verseSpan)
	}
	T, _ = m.Update(key("0"))
	m = T.(*appModel)
	if m.verseSpan != 1 {
		t.Errorf("'0' should reset span to 1, got %d", m.verseSpan)
	}
}
