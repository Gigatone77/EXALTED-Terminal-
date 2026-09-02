package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gigatone/biblelearn/internal/books"
	"github.com/gigatone/biblelearn/internal/engine"
	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/store"
	"github.com/gigatone/biblelearn/internal/textclean"
)

// tab identifiers.
const (
	tabBrowse = iota
	tabSearch
	tabMemory
	tabNotes
)

type appModel struct {
	e   *engine.Engine
	err error

	width  int
	height int
	tab    int

	activeVersion string

	// Browse state.
	browseBooks []books.Book
	book        int // ordinal
	chapter     int
	verse       int
	verseSpan   int // number of consecutive verses shown at once
	bc          *model.BookContent

	// Search state.
	searchInput textinput.Model
	results     []store.SearchResult

	// Memory state.
	cards   []store.MemoryState
	cardIdx int
	shown   bool // show answer
	quality int

	// Notes state.
	notes  []store.Note
	noteCh int // challenge index for navigation

	// messages / toast
	msg    string
	msgTab int
}

func newAppModel(e *engine.Engine) (*appModel, error) {
	m := &appModel{e: e, activeVersion: "KJV"}
	active, err := e.ActiveVersionID()
	if err == nil && active != "" {
		m.activeVersion = active
	}
	m.searchInput = textinput.New()
	m.searchInput.Placeholder = "search the scriptures…"
	m.searchInput.PromptStyle = styleMuted
	m.verseSpan = 1
	// Load books for browse.
	vs, err := e.Store.ImportedBooks(m.activeVersion, "bible")
	if err == nil {
		for _, b := range books.Books() {
			if vs[b.Ordinal] {
				m.browseBooks = append(m.browseBooks, b)
			}
		}
	}
	if len(m.browseBooks) > 0 {
		m.loadBook(m.browseBooks[0].Ordinal)
	}
	return m, nil
}

// loadBook refreshes the browse book content and resets the chapter/verse.
func (m *appModel) loadBook(ord int) {
	m.book = ord
	m.bc, _, _ = m.e.Store.GetBook(m.activeVersion, "bible", ord)
	chapters := m.chapterList()
	if len(chapters) > 0 {
		m.chapter = chapters[0]
	} else {
		m.chapter = 1
	}
	m.verse = 1
}

func (m *appModel) chapterList() []int {
	if m.bc == nil {
		return nil
	}
	return m.bc.ChapterOrder
}

func (m *appModel) verseList() []int {
	if m.bc == nil {
		return nil
	}
	if ch, ok := m.bc.Chapters[m.chapter]; ok {
		return ch.Order
	}
	return nil
}

func (m *appModel) currentVerses() ([]int, bool) {
	if m.bc == nil {
		return nil, false
	}
	ch, ok := m.bc.Chapters[m.chapter]
	if !ok {
		return nil, false
	}
	return ch.Order, true
}

func (m *appModel) verseText(chapter, verse int) string {
	if m.bc == nil {
		return ""
	}
	if ch, ok := m.bc.Chapters[chapter]; ok {
		return textclean.HTML(ch.Verses[verse])
	}
	return ""
}

// Init implements tea.Model.
func (m appModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model.
func (m *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch m.tab {
	case tabSearch:
		if key == "esc" {
			m.tab = tabBrowse
			return m, nil
		}
		if key == "enter" {
			m.runSearch()
			return m, nil
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	case tabMemory:
		return m.handleMemoryKey(key), nil
	case tabNotes:
		return m.handleNotesKey(key), nil
	}
	// Browse tab (and global tab switching).
	switch key {
	case "1":
		m.tab = tabBrowse
	case "2":
		m.tab = tabSearch
		m.searchInput.Focus()
	case "3":
		m.tab = tabMemory
		m.reloadCards()
		m.shown = false
	case "4":
		m.tab = tabNotes
		m.reloadNotes()
	case "left", "h":
		return m, nil
	}
	return m.handleBrowseKey(key), nil
}

func (m *appModel) runSearch() {
	q := m.searchInput.Value()
	if strings.TrimSpace(q) == "" {
		m.msg = "type a query and press enter"
		return
	}
	res, err := m.e.Store.Search(q, m.activeVersion, "", 40)
	if err != nil {
		m.err = err
		return
	}
	m.results = res
}

func (m *appModel) reloadCards() {
	m.cards, _ = m.e.Store.DueCards(m.activeVersion, 100)
	m.cardIdx = 0
	m.shown = false
}

func (m *appModel) reloadNotes() {
	m.notes, _ = m.e.Store.ListNotes(m.activeVersion)
}

func (m appModel) bookName(ord int) string {
	if b, ok := books.ByOrdinal(ord); ok {
		return b.Name
	}
	return fmt.Sprintf("#%d", ord)
}

func (m appModel) currentRef() model.Ref {
	return model.Ref{VersionID: m.activeVersion, Collection: "bible", Book: m.book, Chapter: m.chapter, Verse: m.verse}
}

// studyVerse queues the current verse into the spaced-repetition memory deck.
func (m *appModel) studyVerse() {
	if m.bc == nil {
		return
	}
	st := store.MemoryState{
		VersionID: m.activeVersion, Book: m.book, Chapter: m.chapter, Verse: m.verse,
		IntervalDays: 0, Ease: 2.5, Reps: 0, State: 0, Due: time.Now(),
	}
	if err := m.e.Store.SaveMemory(st); err != nil {
		m.err = err
		return
	}
	m.msg = fmt.Sprintf("Added %s to your memory deck.", m.currentRef().String())
}

func refRef(m store.MemoryState) model.Ref {
	return model.Ref{VersionID: m.VersionID, Collection: "bible", Book: m.Book, Chapter: m.Chapter, Verse: m.Verse}
}

func noteRef(n store.Note) model.Ref {
	return model.Ref{VersionID: n.VersionID, Collection: "bible", Book: n.Book, Chapter: n.Chapter, Verse: n.Verse}
}
