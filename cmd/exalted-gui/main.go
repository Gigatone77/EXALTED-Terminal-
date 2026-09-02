// Command exalted-gui is the graphical (Fyne) edition of EXALTED Terminal.
// It shares the same offline engine and data as the terminal edition, so a
// passage studied here is available on the command line and vice versa.
package main

import (
	"context"
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/gigatone/biblelearn/internal/books"
	"github.com/gigatone/biblelearn/internal/engine"
	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/store"
)

const (
	bgColor = "#0d1117"
	fgColor = "#e6edf3"
	amber   = "#f5a623"
)

type guiApp struct {
	eng *engine.Engine
	w   fyne.Window

	// Browse
	bookSel        *widget.Select
	chapEntry      *widget.Entry
	verseList      *widget.List
	narration      *widget.Label
	refLabel       *widget.Label
	verseSpanLabel *widget.Label
	verseSpan      int   // how many consecutive verses to show (default 1)
	verseSelIdx    int   // currently selected row in verseList
	verses         []int // ordered verse numbers of the current chapter
	bc             *model.BookContent
	curBook        int
	curChap        int

	// Search
	searchEntry *widget.Entry
	searchList  *widget.List
	results     []store.SearchResult

	// Memory
	memText    *widget.Label
	memStat    *widget.Label
	memButtons []*widget.Button
	reveal     bool
	cards      []store.MemoryState
	curCard    store.MemoryState

	// Notes
	notesList *widget.List
	notes     []store.Note
	noteEntry *widget.Entry
}

func main() {
	e, err := engine.New("")
	if err != nil {
		fmt.Println("open engine:", err)
		return
	}
	defer e.Close()
	if err := e.EnsureDefaultVersion(context.Background()); err != nil {
		fmt.Println("seed:", err)
		return
	}

	a := app.New()
	a.Settings().SetTheme(&darkTheme{})
	g := &guiApp{eng: e, verseSpan: 1}
	g.w = a.NewWindow("EXALTED Terminal")

	title := canvas.NewText("EXALTED Terminal", mustColor(amber))
	title.TextSize = 28
	title.TextStyle = fyne.TextStyle{Bold: true}

	top := container.NewCenter(title)

	tabs := container.NewAppTabs(
		container.NewTabItem("Browse", g.buildBrowse()),
		container.NewTabItem("Search", g.buildSearch()),
		container.NewTabItem("Memory", g.buildMemory()),
		container.NewTabItem("Notes", g.buildNotes()),
	)
	tabs.OnSelected = func(ti *container.TabItem) {
		switch ti.Text {
		case "Search":
			g.refreshSearch()
		case "Memory":
			g.reloadCards()
		case "Notes":
			g.reloadNotes()
		}
	}

	g.w.SetContent(container.NewBorder(top, nil, nil, nil, tabs))
	g.w.Resize(fyne.NewSize(1000, 720))
	g.openVerse(43, 3, 16) // John 3:16 by default
	g.w.ShowAndRun()
}

// ---------- Browse ----------

func (g *guiApp) buildBrowse() fyne.CanvasObject {
	names := make([]string, 0, 66)
	for _, b := range books.Books() {
		names = append(names, b.Name)
	}
	g.bookSel = widget.NewSelect(names, func(name string) {
		if b, ok := books.Resolve(name); ok {
			g.openVerse(b.Ordinal, g.curChap, 1)
		}
	})
	g.chapEntry = widget.NewEntry()
	g.chapEntry.SetPlaceHolder("chapter")
	chapBtnMove := widget.NewButton("Go", func() {
		n, _ := strconv.Atoi(strings.TrimSpace(g.chapEntry.Text))
		if n < 1 {
			n = 1
		}
		g.openVerse(g.curBook, n, 1)
	})
	prev := widget.NewButton("◀", func() { g.openVerse(g.curBook, g.curChap-1, 1) })
	next := widget.NewButton("▶", func() { g.openVerse(g.curBook, g.curChap+1, 1) })

	g.refLabel = widget.NewLabel("")
	g.refLabel.TextStyle = fyne.TextStyle{Bold: true}

	nav := container.NewHBox(prev, g.bookSel, g.chapEntry, chapBtnMove, next)

	g.verseList = widget.NewList(
		func() int { return len(g.verses) },
		func() fyne.CanvasObject { return widget.NewLabel("000") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id < len(g.verses) {
				o.(*widget.Label).SetText(strconv.Itoa(g.verses[id]))
			}
			o.(*widget.Label).Alignment = fyne.TextAlignCenter
		},
	)
	g.verseList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(g.verses) {
			g.verseSelIdx = int(id)
			g.selectVerse(id)
		}
	}

	g.narration = widget.NewLabel("")
	g.narration.Wrapping = fyne.TextWrapWord

	// Visible verse-span control: [−] count [+]  1 (reset)
	g.verseSpanLabel = widget.NewLabel(strconv.Itoa(g.verseSpan))
	g.verseSpanLabel.Alignment = fyne.TextAlignCenter
	g.verseSpanLabel.TextStyle = fyne.TextStyle{Bold: true}
	spanMinus := widget.NewButton("−", func() { g.setVerseSpan(g.verseSpan - 1) })
	spanPlus := widget.NewButton("+", func() { g.setVerseSpan(g.verseSpan + 1) })
	spanReset := widget.NewButton("1", func() { g.setVerseSpan(1) })
	spanRow := container.NewHBox(
		widget.NewLabel("verses:"),
		spanMinus,
		g.verseSpanLabel,
		spanPlus,
		spanReset,
	)

	left := container.NewVScroll(g.verseList)
	right := container.NewBorder(
		container.NewVBox(g.refLabel, spanRow),
		nil, nil, nil,
		container.NewVScroll(g.narration),
	)
	split := container.NewHSplit(left, right)
	split.SetOffset(0.20)

	return container.NewBorder(nav, nil, nil, nil, split)
}

func (g *guiApp) openVerse(book, chap, verse int) {
	active, err := g.eng.ActiveVersionID()
	if err != nil {
		return
	}
	bc, ok, err := g.eng.Store.GetBook(active, "bible", book)
	if err != nil || !ok || bc == nil {
		g.narration.SetText("Text not available for this book yet.")
		return
	}
	g.bc = bc
	g.curBook = book
	if b, ok2 := books.ByOrdinal(book); ok2 {
		g.bookSel.SetSelected(b.Name)
	}
	// Clamp chapter into the book's chapter list.
	if len(cOrder(g.bc)) == 0 {
		g.narration.SetText("No chapters present.")
		return
	}
	if chap < 1 {
		chap = 1
	}
	if chap > cOrder(g.bc)[len(cOrder(g.bc))-1] {
		chap = cOrder(g.bc)[len(cOrder(g.bc))-1]
	}
	g.curChap = chap
	g.chapEntry.Text = strconv.Itoa(chap)
	g.verses = chapterVerses(g.bc, chap)
	g.verseList.Refresh()
	if len(g.verses) == 0 {
		g.narration.SetText("No verses in chapter " + strconv.Itoa(chap) + ".")
		return
	}
	if verse < 1 {
		verse = 1
	}
	// Find index of desired verse (defaults to first).
	target := 0
	for i, v := range g.verses {
		if v == verse {
			target = i
			break
		}
	}
	g.verseSelIdx = target
	g.verseList.Select(target)
	g.selectVerse(target)
}

func (g *guiApp) selectVerse(id int) {
	if g.verses == nil || g.narration == nil || g.eng == nil || id < 0 || id >= len(g.verses) {
		return
	}
	vnum := g.verses[id]
	active, _ := g.eng.ActiveVersionID()

	span := g.verseSpan
	if span < 1 {
		span = 1
	}
	b, bOK := books.ByOrdinal(g.curBook)
	bname := ""
	if bOK {
		bname = b.Name
	}

	// Build a passage spanning span consecutive verses from the selected one.
	var sb strings.Builder
	last := id + span - 1
	if last >= len(g.verses) {
		last = len(g.verses) - 1
	}
	for i := id; i <= last; i++ {
		v := g.verses[i]
		txt, ok, _ := g.eng.Store.VerseText(active, model.Ref{
			VersionID: active, Collection: "bible", Book: g.curBook, Chapter: g.curChap, Verse: v,
		})
		if i > id {
			sb.WriteString("\n\n")
		}
		if ok && strings.TrimSpace(txt) != "" {
			fmt.Fprintf(&sb, "▸ %d  %s", v, txt)
		} else {
			fmt.Fprintf(&sb, "▸ %d", v)
		}
	}
	if sb.Len() == 0 {
		g.narration.SetText("No text.")
	} else {
		g.narration.SetText(sb.String())
	}
	start := vnum
	end := g.verses[last]
	if bname != "" {
		if end == start {
			g.refLabel.SetText(fmt.Sprintf("📖 %s %d:%d", bname, g.curChap, start))
		} else {
			g.refLabel.SetText(fmt.Sprintf("📖 %s %d:%d–%d", bname, g.curChap, start, end))
		}
	}
	if g.verseSpanLabel != nil {
		g.verseSpanLabel.SetText(strconv.Itoa(g.verseSpan))
	}
}

// setVerseSpan adjusts how many consecutive verses the Browse pane shows,
// clamped to at least 1, then re-renders the passage.
func (g *guiApp) setVerseSpan(n int) {
	if n < 1 {
		n = 1
	}
	if len(g.verses) > 0 && n > len(g.verses) {
		n = len(g.verses)
	}
	g.verseSpan = n
	if g.verseSpanLabel != nil {
		g.verseSpanLabel.SetText(strconv.Itoa(n))
	}
	// Re-render for the currently selected verse.
	if g.verseSelIdx >= 0 && g.verseSelIdx < len(g.verses) {
		g.selectVerse(g.verseSelIdx)
	}
}

// cOrder returns the stable chapter list of a book.
func cOrder(bc *model.BookContent) []int {
	if bc == nil {
		return nil
	}
	if len(bc.ChapterOrder) > 0 {
		return bc.ChapterOrder
	}
	var out []int
	for c := range bc.Chapters {
		out = append(out, c)
	}
	return out
}

func chapterVerses(bc *model.BookContent, chap int) []int {
	if bc == nil {
		return nil
	}
	ch, ok := bc.Chapters[chap]
	if !ok || ch == nil {
		return nil
	}
	return ch.Order
}

// ---------- Search ----------

func (g *guiApp) buildSearch() fyne.CanvasObject {
	g.searchEntry = widget.NewEntry()
	g.searchEntry.SetPlaceHolder("Search all installed versions…")
	g.searchEntry.OnSubmitted = func(string) { g.runSearch() }
	btn := widget.NewButton("Search", func() { g.runSearch() })
	g.searchList = widget.NewList(
		func() int { return len(g.results) },
		func() fyne.CanvasObject {
			l := widget.NewLabel("result")
			l.Wrapping = fyne.TextWrapWord
			return l
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(g.results) {
				return
			}
			r := g.results[id]
			o.(*widget.Label).SetText(fmt.Sprintf("%s %d:%d (%s)\n%s",
				r.BookName, r.Chapter, r.Verse, r.VersionID, r.Text))
		},
	)
	g.searchList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(g.results) {
			r := g.results[id]
			g.openVerse(r.Book, r.Chapter, r.Verse)
		}
	}
	return container.NewBorder(container.NewHBox(g.searchEntry, btn), nil, nil, nil, g.searchList)
}

func (g *guiApp) runSearch() {
	q := strings.TrimSpace(g.searchEntry.Text)
	if q == "" {
		g.results = nil
		g.searchList.Refresh()
		return
	}
	active, _ := g.eng.ActiveVersionID()
	results, err := g.eng.Store.Search(q, "", "", 200)
	if err != nil {
		g.results = nil
		g.searchList.Refresh()
		return
	}
	// Prefer the active version; fall back to all matches.
	var filtered []store.SearchResult
	for _, r := range results {
		if r.VersionID == active {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		filtered = results
	}
	g.results = filtered
	g.searchList.Refresh()
}

func (g *guiApp) refreshSearch() {}

// ---------- Memory ----------

func (g *guiApp) buildMemory() fyne.CanvasObject {
	g.memStat = widget.NewLabel("")
	g.memText = widget.NewLabel("")
	g.memText.Wrapping = fyne.TextWrapWord
	again := widget.NewButton("Again (0)", func() { g.grade(0) })
	hard := widget.NewButton("Hard (3)", func() { g.grade(3) })
	good := widget.NewButton("Good (4)", func() { g.grade(4) })
	easy := widget.NewButton("Easy (5)", func() { g.grade(5) })
	g.memButtons = []*widget.Button{again, hard, good, easy}
	for _, b := range g.memButtons {
		b.Disable()
	}
	row := container.NewHBox(again, hard, good, easy)
	return container.NewBorder(container.NewVBox(g.memStat), nil, nil,
		row, container.NewVScroll(g.memText))
}

func (g *guiApp) reloadCards() {
	active, _ := g.eng.ActiveVersionID()
	cards, err := g.eng.Store.DueCards(active, 50)
	if err != nil {
		return
	}
	g.cards = cards
	_, due, learning, review, _ := g.eng.Store.MemoryStats(active)
	g.memStat.SetText(fmt.Sprintf("Deck · %d due · %d learning · %d review", due, learning, review))
	if len(cards) == 0 {
		g.memText.SetText("Nothing due right now. Open a verse on the Browse tab, then note it to add it to your deck.")
		g.setMemoryButtons(false)
		_ = review
		return
	}
	g.curCard = cards[0]
	g.reveal = false
	g.refreshCard()
}

func (g *guiApp) refreshCard() {
	c := g.curCard
	ref := fmt.Sprintf("%s %d:%d", verseBookName(c.Book), c.Chapter, c.Verse)
	text, ok, _ := g.eng.Store.VerseText(c.VersionID, model.Ref{
		VersionID: c.VersionID, Collection: "bible", Book: c.Book, Chapter: c.Chapter, Verse: c.Verse,
	})
	if ok {
		text = ref + "\n\n" + text + "\n\n(Share normally to memorize.)"
	}
	if g.reveal {
		g.memText.SetText(text)
		g.setMemoryButtons(true)
	} else {
		g.memText.SetText(ref + "\n\nPress… (remember the text, then grade)")
		g.setMemoryButtons(false)
	}
}

func (g *guiApp) grade(q int) {
	if !g.reveal {
		g.reveal = true
		g.refreshCard()
		return
	}
	updated := store.ScheduleNext(g.curCard, q)
	if err := g.eng.Store.SaveMemory(updated); err != nil {
		return
	}
	g.cards = g.cards[1:]
	g.reveal = false
	if len(g.cards) == 0 {
		g.cards = nil
		g.reloadCards()
		return
	}
	g.curCard = g.cards[0]
	g.refreshCard()
}

func (g *guiApp) setMemoryButtons(on bool) {
	for _, b := range g.memButtons {
		if on {
			b.Enable()
		} else {
			b.Disable()
		}
	}
}

// ---------- Notes ----------

func (g *guiApp) buildNotes() fyne.CanvasObject {
	g.notesList = widget.NewList(
		func() int { return len(g.notes) },
		func() fyne.CanvasObject {
			l := widget.NewLabel("note")
			l.Wrapping = fyne.TextWrapWord
			return l
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(g.notes) {
				return
			}
			o.(*widget.Label).SetText(g.noteRef(g.notes[id]))
		},
	)
	g.notesList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(g.notes) {
			n := g.notes[id]
			g.openVerse(n.Book, n.Chapter, n.Verse)
		}
	}
	g.noteEntry = widget.NewEntry()
	g.noteEntry.SetPlaceHolder("Write a note about the selected verse…")
	g.noteEntry.MultiLine = true
	saveBtn := widget.NewButton("Save note", func() {
		active, _ := g.eng.ActiveVersionID()
		body := strings.TrimSpace(g.noteEntry.Text)
		if body == "" || g.currentChapterVerseCount() == 0 {
			return
		}
		_ = g.eng.Store.SaveNote(store.Note{
			VersionID: active, Book: g.curBook, Chapter: g.curChap, Verse: g.currentVerse(), Body: body,
		})
		g.noteEntry.SetText("")
		g.reloadNotes()
	})
	head := container.NewBorder(nil, nil, nil, nil, g.noteEntry)
	_ = head
	return container.NewBorder(container.NewVBox(g.noteEntry, saveBtn, widget.NewLabel("")),
		nil, nil, nil, g.notesList)
}

func (g *guiApp) reloadNotes() {
	active, _ := g.eng.ActiveVersionID()
	g.notes, _ = g.eng.Store.ListNotes(active)
	g.notesList.Refresh()
}

func (g *guiApp) noteRef(n store.Note) string {
	return fmt.Sprintf("%s %d:%d\n%s", verseBookName(n.Book), n.Chapter, n.Verse, n.Body)
}

func (g *guiApp) currentChapterVerseCount() int {
	return len(g.verses)
}

func (g *guiApp) currentVerse() int {
	if len(g.verses) == 0 {
		return 1
	}
	return g.verses[0]
}

func verseBookName(ord int) string {
	if b, ok := books.ByOrdinal(ord); ok {
		return b.Name
	}
	return "?"
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mustColor(hex string) color.Color {
	c, err := parseHex(hex)
	if err != nil {
		return color.RGBA{R: 13, G: 17, B: 23, A: 0xff}
	}
	return c
}

func parseHex(hex string) (color.RGBA, error) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return color.RGBA{}, fmt.Errorf("bad hex: %q", hex)
	}
	var v uint32
	for i := 0; i < 6; i++ {
		c := hex[i]
		var val uint32
		switch {
		case c >= '0' && c <= '9':
			val = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			val = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			val = uint32(c-'A') + 10
		default:
			return color.RGBA{}, fmt.Errorf("bad hex char %q", c)
		}
		v = v<<4 | val
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8 & 0xff), B: uint8(v & 0xff), A: 0xff}, nil
}

// ---------- Theme ----------

type darkTheme struct{}

func (d *darkTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground, theme.ColorNameInputBackground, theme.ColorNameInputBorder:
		return mustColor(bgColor)
	case theme.ColorNameForeground:
		return mustColor(fgColor)
	case theme.ColorNamePrimary:
		return mustColor(amber)
	case theme.ColorNameHover:
		return mustColor("#1c2128")
	case theme.ColorNameSeparator:
		return mustColor("#21262d")
	case theme.ColorNamePlaceHolder:
		return mustColor("#8b949e")
	default:
		return theme.DarkTheme().Color(name, variant)
	}
}
func (d *darkTheme) Font(style fyne.TextStyle) fyne.Resource { return theme.DarkTheme().Font(style) }
func (d *darkTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DarkTheme().Icon(name)
}
func (d *darkTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DarkTheme().Size(name)
}
