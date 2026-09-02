package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View implements tea.Model. It renders the current tab.
func (m appModel) View() string {
	if m.err != nil {
		return styleWarnBox().Render(fmt.Sprintf("error: %v", m.err))
	}
	var body string
	switch m.tab {
	case tabSearch:
		body = m.viewSearch()
	case tabMemory:
		body = m.viewMemory()
	case tabNotes:
		body = m.viewNotes()
	default:
		body = m.viewBrowse()
	}
	return styleBase.Render(
		styleTitle.Render("EXALTED Terminal") + "\n" +
			m.toolbar() + "\n\n" +
			body + "\n" +
			m.footer(),
	)
}

func (m appModel) viewBrowse() string {
	version := styleMuted.Render(fmt.Sprintf("(%s)", m.activeVersion))
	var sb strings.Builder
	sb.WriteString(styleTitle.Render(fmt.Sprintf("%s %s", m.bookName(m.book), version)))
	if m.bc == nil {
		sb.WriteString("\n\n  No text loaded for this version.")
		return sb.String()
	}
	sb.WriteString("\n")
	// Chapter/verse line.
	total := m.versesInChapter()
	last := m.verse + m.verseSpan - 1
	if last > total {
		last = total
	}
	rangeLabel := fmt.Sprintf("%d:%d", m.chapter, m.verse)
	if last != m.verse {
		rangeLabel += fmt.Sprintf("–%d", last)
	}
	sb.WriteString(fmt.Sprintf("  %s%s%s\n\n",
		styleMuted.Render("Chapter "), rangeLabel, styleDim.Render(" / "+strconv.Itoa(m.verseSpan)+" of "+strconv.Itoa(total))))

	// Visible verse-span control in the reading area: [−] N [+] · 0 reset
	// so the expand/shrink action is discoverable right where you read.
	sb.WriteString(fmt.Sprintf("  %s%s%s   %s\n\n",
		styleMuted.Render("verses:"),
		styleDim.Render("  [  -"), styleAccent().Render(fmt.Sprintf(" %d ", m.verseSpan)),
		styleDim.Render("+ ]  0 = reset to 1")))

	// Verse display: the whole visible span (larger, wrapped).
	for v := m.verse; v <= last; v++ {
		head := styleAccent().Render(fmt.Sprintf("▸ %d", v))
		if v > m.verse {
			head = styleMuted.Render(fmt.Sprintf("▸ %d", v))
		}
		if v == m.verse {
			if _, exists, _ := m.e.Store.GetNote(m.activeVersion, m.book, m.chapter, v); exists {
				head += styleMuted.Render("  ✎ note")
			}
		}
		sb.WriteString(head + " " + styleBody.Render(m.verseText(m.chapter, v)) + "\n\n")
	}

	// Chapter overview (small book/chapter progress).
	if notesCount := m.noteCount(); notesCount > 0 {
		sb.WriteString(styleMuted.Render(fmt.Sprintf("%d study note(s) in this book", notesCount)))
	}
	return sb.String()
}

func (m appModel) versesInChapter() int {
	vs, _ := m.currentVerses()
	return len(vs)
}

func (m appModel) noteCount() int {
	n, _ := m.e.Store.ListNotes(m.activeVersion)
	c := 0
	for _, note := range n {
		if note.Book == m.book {
			c++
		}
	}
	return c
}

func (m appModel) viewSearch() string {
	var sb strings.Builder
	sb.WriteString(styleTitle.Render("Search"))
	sb.WriteString("\n\n  " + m.searchInput.View() + "\n")
	if len(m.results) == 0 && strings.TrimSpace(m.searchInput.Value()) != "" {
		sb.WriteString("\n  No results.")
		return sb.String()
	}
	for _, r := range m.results {
		ref := fmt.Sprintf("%s %d:%d", m.bookName(r.Book), r.Chapter, r.Verse)
		sb.WriteString("\n" + styleAccent().Render(ref) + " " + styleMuted.Render(r.VersionID))
		sb.WriteString("\n  " + styleBody.Render(truncate(r.Text, 160)) + "\n")
	}
	return sb.String()
}

func (m appModel) viewMemory() string {
	var sb strings.Builder
	sb.WriteString(styleTitle.Render("Memory Review"))
	if len(m.cards) == 0 {
		sb.WriteString("\n\n  " + styleMuted.Render("Nothing due right now. Great work."))
		return sb.String()
	}
	card := m.cards[m.cardIdx]
	ref := fmt.Sprintf("%s %d:%d", m.bookName(card.Book), card.Chapter, card.Verse)
	sb.WriteString(fmt.Sprintf("\n\n  %s  %s\n\n",
		styleMuted.Render(fmt.Sprintf("card %d/%d", m.cardIdx+1, len(m.cards))),
		styleAccent().Render(ref)))
	text, _, _ := m.e.Store.VerseText(m.activeVersion, refRef(card))
	if m.shown {
		sb.WriteString(styleBody.Render(text) + "\n\n")
		sb.WriteString(styleMuted.Render("  a again · h hard · g good · e easy"))
	} else {
		sb.WriteString("\n  " + styleDim.Render("(press space to reveal)") + "\n")
	}
	return sb.String()
}

func (m appModel) viewNotes() string {
	var sb strings.Builder
	sb.WriteString(styleTitle.Render("Study Notes"))
	if len(m.notes) == 0 {
		sb.WriteString("\n\n  " + styleMuted.Render("No notes yet. Open a verse on the Browse tab and press c."))
		return sb.String()
	}
	for i, n := range m.notes {
		mark := " "
		if i == m.noteCh {
			mark = "▶"
		}
		ref := fmt.Sprintf("%s %d:%d", m.bookName(n.Book), n.Chapter, n.Verse)
		sb.WriteString(fmt.Sprintf("\n  %s %s\n", mark, ref))
		text, _, _ := m.e.Store.VerseText(m.activeVersion, noteRef(n))
		if text != "" {
			sb.WriteString("    " + styleMuted.Render(truncate(text, 90)) + "\n")
		}
	}
	return sb.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func styleWarnBox() lipgloss.Style { return styleBox.Foreground(tWarn) }
