package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gigatone/biblelearn/internal/books"
	"github.com/gigatone/biblelearn/internal/store"
)

// handleBrowseKey processes keyboard input on the Browse tab: book/chapter/verse
// navigation plus note toggling.
func (m *appModel) handleBrowseKey(key string) tea.Model {
	switch key {
	case "up":
		// Shrink the visible verse span (show fewer lines).
		m.setVerseSpan(m.verseSpan - 1)
	case "down":
		// Expand the visible verse span (show more lines).
		m.setVerseSpan(m.verseSpan + 1)
	case "left", "h":
		// Flip back a full page (verseSpan verses).
		m.pageBackward()
	case "right", "l":
		// Flip forward a full page (verseSpan verses).
		m.pageForward()
	case "n":
		// next chapter
		if b, ok := books.ByOrdinal(m.book); ok && m.chapter < b.Chapters {
			m.chapter++
			m.verse = 1
		}
	case "p":
		// previous book
		m.moveBook(-1)
	case "o":
		// next book
		m.moveBook(1)
	case "c":
		m.addNote()
	case "s":
		m.studyVerse()
	case "]", "=", "+":
		// expand the visible verse span
		m.setVerseSpan(m.verseSpan + 1)
	case "[", "-", "_":
		// contract the visible verse span
		m.setVerseSpan(m.verseSpan - 1)
	case "0":
		// reset to a single verse
		m.setVerseSpan(1)
	}
	return m
}

// setVerseSpan adjusts how many consecutive verses are shown, clamped to at
// least 1 and to the remaining verses in the chapter.
func (m *appModel) setVerseSpan(n int) {
	vs, _ := m.currentVerses()
	total := len(vs)
	if n < 1 {
		n = 1
	}
	// Don't let the span push past the end of the chapter.
	maxSpan := total - m.verse + 1
	if maxSpan < 1 {
		maxSpan = 1
	}
	if n > maxSpan {
		n = maxSpan
	}
	m.verseSpan = n
	m.msg = fmt.Sprintf("Showing %d verse(s) from %s.", m.verseSpan, m.currentRef().String())
}

// pageForward flips a page proportional to the current expansion: it advances
// by verseSpan verses at once (the whole visible page), spilling into the next
// chapter when the current one runs out.
func (m *appModel) pageForward() {
	vs, _ := m.currentVerses()
	total := len(vs)
	if m.verse+m.verseSpan <= total {
		m.verse += m.verseSpan
		return
	}
	if b, ok := books.ByOrdinal(m.book); ok && m.chapter < b.Chapters {
		m.chapter++
		m.verse = 1
	} else if m.verse < total {
		m.verse = total
	}
}

// pageBackward flips back a full visible page (verseSpan verses), landing on
// the last full page of the previous chapter when the current one starts.
func (m *appModel) pageBackward() {
	if m.verse-m.verseSpan >= 1 {
		m.verse -= m.verseSpan
		return
	}
	if m.chapter > 1 {
		m.chapter--
		if vs, _ := m.currentVerses(); len(vs) > m.verseSpan {
			m.verse = len(vs) - m.verseSpan + 1
		} else {
			m.verse = 1
		}
	} else if m.verse > 1 {
		m.verse = 1
	}
}

func (m *appModel) moveBook(delta int) {
	if len(m.browseBooks) == 0 {
		return
	}
	idx := -1
	for i, b := range m.browseBooks {
		if b.Ordinal == m.book {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	}
	idx += delta
	if idx < 0 {
		idx = len(m.browseBooks) - 1
	}
	if idx >= len(m.browseBooks) {
		idx = 0
	}
	m.loadBook(m.browseBooks[idx].Ordinal)
}

// handleMemoryKey processes the Memory review tab.
func (m *appModel) handleMemoryKey(key string) tea.Model {
	switch key {
	case "q", "esc":
		m.tab = tabBrowse
		return m
	case " ":
		m.shown = true
	case "a", "0", "1":
		if m.shown {
			m.grade(0) // again
		}
	case "h", "3":
		if m.shown {
			m.grade(3) // hard
		}
	case "g", "4":
		if m.shown {
			m.grade(4) // good
		}
	case "e", "5":
		if m.shown {
			m.grade(5) // easy
		}
	}
	return m
}

func (m *appModel) grade(q int) {
	if m.cardIdx >= len(m.cards) {
		return
	}
	card := m.cards[m.cardIdx]
	next := store.ScheduleNext(card, q)
	_ = m.e.Store.SaveMemory(next)
	m.cardIdx++
	if m.cardIdx >= len(m.cards) {
		m.msg = "session complete — nothing left due."
		m.cardIdx = 0
	}
	m.shown = false
}

// handleNotesKey processes the Notes tab.
func (m *appModel) handleNotesKey(key string) tea.Model {
	switch key {
	case "q", "esc":
		m.tab = tabBrowse
	case "up":
		if m.noteCh > 0 {
			m.noteCh--
		}
	case "down":
		if m.noteCh < len(m.notes)-1 {
			m.noteCh++
		}
	case "d", "x":
		if m.noteCh >= 0 && m.noteCh < len(m.notes) {
			n := m.notes[m.noteCh]
			_ = m.e.Store.SaveNote(store.Note{VersionID: n.VersionID, Book: n.Book, Chapter: n.Chapter, Verse: n.Verse, Body: ""})
			m.reloadNotes()
			if m.noteCh >= len(m.notes) {
				m.noteCh = len(m.notes) - 1
			}
		}
	}
	return m
}

func (m *appModel) addNote() {
	if m.bc == nil {
		return
	}
	n := store.Note{VersionID: m.activeVersion, Book: m.book, Chapter: m.chapter, Verse: m.verse}
	_, exists, _ := m.e.Store.GetNote(m.activeVersion, m.book, m.chapter, m.verse)
	if exists {
		return
	}
	_ = m.e.Store.SaveNote(n)
	m.msg = fmt.Sprintf("Note added for %s.", m.currentRef().String())
}

// toolbar renders the tab bar.
func (m appModel) toolbar() string {
	tabs := []struct {
		label string
		key   string
	}{
		{"Browse", "1"}, {"Search", "2"}, {"Memory", "3"}, {"Notes", "4"},
	}
	var b strings.Builder
	for i, t := range tabs {
		label := fmt.Sprintf(" %s %s ", t.key, t.label)
		var s string
		if i == m.tab {
			s = styleTabSel.Render(label)
		} else {
			s = styleTabIdle.Render(label)
		}
		b.WriteString(s)
	}
	return b.String()
}

// footer renders hint text plus a toast message if present.
func (m appModel) footer() string {
	var hints string
	switch m.tab {
	case tabBrowse:
		hints = "↓ expand ↑ shrink · ←→ page flips (span-sized) · n chapter · p/o books · 0 span=1 · c note · s memorize · 1-4 tabs"
	case tabSearch:
		hints = "enter search · esc back"
	case tabMemory:
		hints = "space reveal · a=0 h=3 g=4 e=5 · q quit"
	case tabNotes:
		hints = "↑↓ select · d delete · q back"
	}
	if m.msg != "" {
		hints = styleAccent().Render(m.msg) + "  |  " + hints
	}
	return styleHint.Render(hints)
}

func styleAccent() lipgloss.Style { return lipgloss.NewStyle().Foreground(tAccentHi).Bold(true) }
