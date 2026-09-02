package pdfimport

import (
	"bytes"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gigatone/biblelearn/internal/books"
	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/store"
)

// bboxWord is a single word with its bounding box from pdftotext -bbox.
type bboxWord struct {
	xMin, yMin float64
	text       string
}

// Result reports the outcome of a whole-Bible import.
type Result struct {
	// Verses is the total number of verses imported.
	Verses int
	// Books is the number of books stored.
	Books int
	// Skipped lists book names that failed verse-count verification and were
	// not stored (quality gate).
	Skipped []string
}

// ImportWholeBibleFromPDF imports a complete Bible from a single multi-page PDF
// that uses the standard NKJV two-column layout (standalone verse-number lines,
// chapter-start verses labeled with the chapter number). It requires the
// poppler "pdftotext" binary for coordinate-based column reconstruction.
//
// Each book must reproduce its canonical verse count; books that fail the
// check are recorded in Result.Skipped and omitted rather than stored with
// corrupt chapter/verse structure.
func ImportWholeBibleFromPDF(s *store.Store, versionID string, pdfPath string, progress func(book string, verses int)) (*Result, error) {
	lines, err := extractColumns(pdfPath)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	builder := newBookBuilder(versionID, "bible")
	for _, ln := range lines {
		builder.feed(ln)
	}
	booksDone := builder.flush()
	for ord, bc := range booksDone {
		vs := countBookVerses(bc)
		if vs == 0 || vs != books.VerseCount(ord) {
			name, _ := books.ByOrdinal(ord)
			res.Skipped = append(res.Skipped, name.Name)
			continue
		}
		if err := s.PutBook(versionID, "bible", ord, bc); err != nil {
			return res, err
		}
		if err := s.IndexBook(versionID, "bible", ord); err != nil {
			return res, err
		}
		res.Books++
		res.Verses += vs
		if progress != nil {
			name, _ := books.ByOrdinal(ord)
			progress(name.Name, vs)
		}
	}
	return res, nil
}

// extractColumns orders every page into left/right columns (by x-gap
// clustering) and returns reading-order "lines". A line is either a standalone
// verse/chapter label (a bare integer) or a text fragment.
func extractColumns(pdfPath string) ([]string, error) {
	var buf bytes.Buffer
	cmd := exec.Command("pdftotext", "-bbox", pdfPath, "-")
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	pages := regexp.MustCompile(`(?s)<page width="([\d.]+)" height="([\d.]+)">(.*?)</page>`).FindAllStringSubmatch(buf.String(), -1)
	var out []string
	for _, pg := range pages {
		words := parseBboxWords(pg[3])
		if len(words) == 0 {
			continue
		}
		cols := clusterColumns(words)
		for ci := range cols {
			w := cols[ci]
			sort.SliceStable(w, func(i, j int) bool {
				if w[i].yMin != w[j].yMin {
					return w[i].yMin < w[j].yMin
				}
				return w[i].xMin < w[j].xMin
			})
			// Group into lines by vertical position.
			lineY := -1.0
			var sb strings.Builder
			for _, wd := range w {
				if lineY < 0 || wd.yMin-lineY > 3.0 {
					if sb.Len() > 0 {
						out = append(out, cleanBboxLine(sb.String()))
						sb.Reset()
					}
					lineY = wd.yMin
				}
				if sb.Len() > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(wd.text)
			}
			if sb.Len() > 0 {
				out = append(out, cleanBboxLine(sb.String()))
			}
		}
	}
	return out, nil
}

func parseBboxWords(seg string) []bboxWord {
	var out []bboxWord
	re := regexp.MustCompile(`<word xMin="([\d.]+)" yMin="([\d.]+)" xMax="[^"]*" yMax="[^"]*">([^<]*)</word>`)
	for _, m := range re.FindAllStringSubmatch(seg, -1) {
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		out = append(out, bboxWord{xMin: x, yMin: y, text: m[3]})
	}
	return out
}

// clusterColumns groups words into 1–2 columns based on horizontal gaps.
func clusterColumns(words []bboxWord) [][]bboxWord {
	const pageW = 595.44
	// Compute a histogram of x-centers to find the dominant gap.
	minX, maxX := pageW, 0.0
	for _, w := range words {
		if w.xMin < minX {
			minX = w.xMin
		}
		if w.xMin > maxX {
			maxX = w.xMin
		}
	}
	// Split at the midpoint of the page if the page clearly has two columns
	// (i.e. some words occupy the right half). Otherwise everything is one
	// column (e.g. a heading page or a full-width passage).
	hasRight := false
	for _, w := range words {
		if w.xMin > pageW/2 {
			hasRight = true
			break
		}
	}
	_ = minX
	_ = maxX
	if !hasRight {
		return [][]bboxWord{words}
	}
	left := make([]bboxWord, 0, len(words))
	right := make([]bboxWord, 0, len(words)/2)
	for _, w := range words {
		if w.xMin < pageW/2 {
			left = append(left, w)
		} else {
			right = append(right, w)
		}
	}
	return [][]bboxWord{left, right}
}

var bboxReplace = strings.NewReplacer(
	"&quot;", "\"", "&apos;", "'", "&amp;", "&", "&lt;", "<", "&gt;", ">",
)

func cleanBboxLine(s string) string {
	s = bboxReplace.Replace(s)
	// Merge a leading inline label like "14" with following text if present.
	return strings.TrimSpace(s)
}

// bookBuilder accumulates lines into book/chapter/verse structure using the
// NKJV layout invariant: within a chapter verses run 1,2,3… and the first verse
// of a new chapter is labeled with the chapter number.
type bookBuilder struct {
	version string
	coll    string

	book        int // current ordinal
	pendingBook int // a heading seen but not yet committed (guards the TOC)
	chapter     int
	expected    int // next verse number expected in current chapter
	curVerses   map[int]string
	curOrder    []int

	open map[int]*model.BookContent
}

func newBookBuilder(version, coll string) *bookBuilder {
	return &bookBuilder{version: version, coll: coll, open: map[int]*model.BookContent{}}
}

var standaloneLabelRe = regexp.MustCompile(`^(\d{1,3})\s*$`)
var leadingVerseRe = regexp.MustCompile(`^(\d{1,3})\s*(\S.*)$`)

func (b *bookBuilder) feed(line string) {
	trim := strings.TrimSpace(line)
	if trim == "" {
		return
	}
	// A known book name on its own line is a heading. Before any book is open
	// this may be the front-matter table of contents, so it becomes a "pending"
	// book that only takes effect when real verse content follows. Once inside a
	// book (past the TOC), a heading is a genuine book boundary.
	if ord := books.ResolveOrdinal(trim); ord != 0 {
		if b.book != 0 {
			b.flushBook()
			b.book = ord
			b.chapter = 0
			b.expected = 1
			b.curVerses = map[int]string{}
			b.curOrder = nil
		} else {
			b.pendingBook = ord
		}
		return
	}

	// Determine whether this line carries a verse label: either a bare number
	// (second-column marker) or a leading number glued to text ("2 The earth…"
	// or "14In the beginning…").
	num := 0
	text := trim
	if m := standaloneLabelRe.FindStringSubmatch(trim); m != nil {
		num, _ = strconv.Atoi(m[1])
		text = ""
	} else if m := leadingVerseRe.FindStringSubmatch(trim); m != nil {
		num, _ = strconv.Atoi(m[1])
		text = strings.TrimSpace(m[2])
	}

	// If we're still before any book, only begin the Bible when real verse
	// content (with a label or text) shows up right after the Genesis heading.
	if b.book == 0 {
		if b.pendingBook == 1 {
			b.book = 1
			b.chapter = 0
			b.expected = 1
			b.curVerses = map[int]string{}
			b.curOrder = nil
			b.pendingBook = 0
		} else {
			return
		}
	}

	if num != 0 {
		b.startVerseByLabel(num, text)
		if text != "" {
			b.appendText(text)
		}
		return
	}

	// Plain text continuation: attach to the current verse, or start verse 1 if
	// none is open yet.
	if len(b.curOrder) > 0 {
		b.appendText(trim)
	} else {
		// No active verse yet: this is the first verse of the book.
		b.ensureChapter()
		b.startVerse(1)
		b.appendText(trim)
	}
}

func (b *bookBuilder) ensureChapter() {
	if b.chapter == 0 {
		b.startChapter(1)
	}
}

func (b *bookBuilder) startChapter(n int) {
	if n <= 0 {
		n = 1
	}
	// Commit the just-finished chapter to its book before moving on.
	if b.book != 0 && len(b.curOrder) > 0 {
		bc := b.bc(b.book)
		ch := &model.Chapter{VersionID: b.version, Collection: b.coll, Book: b.book, Number: b.chapter, Verses: b.curVerses, Order: b.curOrder}
		if _, ok := bc.Chapters[ch.Number]; !ok {
			bc.ChapterOrder = append(bc.ChapterOrder, ch.Number)
		}
		bc.Chapters[ch.Number] = ch
	}
	b.chapter = n
	b.expected = 2
	b.curVerses = map[int]string{}
	b.curOrder = nil
}

func (b *bookBuilder) startVerse(n int) {
	if _, ok := b.curVerses[n]; !ok {
		b.curOrder = append(b.curOrder, n)
		b.curVerses[n] = ""
	}
	b.expected = n + 1
}

// startVerseByLabel applies the NKJV layout invariant to a verse label: a label
// equal to the next expected verse continues the current chapter, otherwise the
// label is the new chapter number (its verse 1).
func (b *bookBuilder) startVerseByLabel(n int, text string) {
	if b.chapter == 0 {
		b.startChapter(1)
	}
	if n != b.expected {
		b.startChapter(n)
		b.startVerse(1)
	} else {
		b.startVerse(n)
	}
	_ = text
}

func (b *bookBuilder) appendText(txt string) {
	if len(b.curOrder) == 0 {
		return
	}
	last := b.curOrder[len(b.curOrder)-1]
	old := b.curVerses[last]
	if old == "" {
		b.curVerses[last] = txt
	} else {
		b.curVerses[last] = strings.TrimSpace(old + " " + txt)
	}
}

func (b *bookBuilder) bc(ord int) *model.BookContent {
	bc, ok := b.open[ord]
	if !ok {
		bc = model.NewBookContent(b.version, b.coll, ord)
		b.open[ord] = bc
	}
	return bc
}

func (b *bookBuilder) flushBook() {
	if b.book == 0 || len(b.curVerses) == 0 {
		return
	}
	bc := b.bc(b.book)
	ch := &model.Chapter{VersionID: b.version, Collection: b.coll, Book: b.book, Number: b.chapter, Verses: b.curVerses, Order: b.curOrder}
	if _, ok := bc.Chapters[ch.Number]; !ok {
		bc.ChapterOrder = append(bc.ChapterOrder, ch.Number)
	}
	bc.Chapters[ch.Number] = ch
	b.curVerses = map[int]string{}
	b.curOrder = nil
	b.chapter = 0
	b.expected = 1
}

func (b *bookBuilder) flush() map[int]*model.BookContent {
	b.flushBook()
	return b.open
}

func countBookVerses(bc *model.BookContent) int {
	n := 0
	for _, ch := range bc.Chapters {
		if ch != nil {
			n += len(ch.Verses)
		}
	}
	return n
}
