// Package pdfimport extracts verse-structured text from Bible book PDFs and
// writes it into the store's version archive. It prefers the high-quality
// column-preserving output of poppler's pdftotext when available, falling back
// to an embedded pure-Go extractor so the binary stays self-contained.
package pdfimport

import (
	"bytes"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/store"
	"github.com/ledongthuc/pdf"
)

// ImportBookFromPDF extracts the text from a book PDF and stores it as the
// given book ordinal in the version/collection. Returns verses parsed.
func ImportBookFromPDF(s *store.Store, versionID, collection string, bookOrdinal int, pdfPath string, progress func(book string, verses int)) (int, error) {
	text, err := extractText(pdfPath)
	if err != nil {
		return 0, err
	}
	bookName := ""
	_ = bookName
	bc, verses := parseBibleBook(versionID, collection, bookOrdinal, text)
	if err := s.PutBook(versionID, collection, bookOrdinal, bc); err != nil {
		return 0, err
	}
	if err := s.IndexBook(versionID, collection, bookOrdinal); err != nil {
		return 0, err
	}
	if progress != nil {
		progress(bc.BookName, verses)
	}
	return verses, nil
}

// extractText returns laid-out text for a PDF, preferring poppler.
func extractText(path string) (string, error) {
	if out, err := pdftotextLayout(path); err == nil && strings.TrimSpace(out) != "" {
		return out, nil
	}
	return embeddedExtract(path)
}

func pdftotextLayout(path string) (string, error) {
	exe, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	cmd := exec.Command(exe, "-layout", path, "-")
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func embeddedExtract(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		t, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(t)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

var (
	chapterRe = regexp.MustCompile(`(?i)(?:^|\s)(chapter|capítulo|capitulo|cap\.|kapitel|第\s*\d+\s*章|باب)\s*[:.\s]*(\d*)|(?:^|\s)(\d+)(?:\.)?\s*(?:chapter|capítulo|capitulo)\b`)
	leadingRe = regexp.MustCompile(`^(\d{1,3})\s+(.+)$`)
	bareNumRe = regexp.MustCompile(`^(\d{1,4})$`)
	titleRe   = regexp.MustCompile(`^\s*$`)
)

// parseBibleBook turns extracted PDF text into a structured book.
func parseBibleBook(versionID, collection string, bookOrdinal int, text string) (*model.BookContent, int) {
	bc := model.NewBookContent(versionID, collection, bookOrdinal)

	chapter := 0
	var ch *model.Chapter
	lastVerse := 0
	nextVerse := 0
	total := 0
	pendingNumOnly := 0 // count consecutive bare-number lines (header guard)

	flushChapter := func() {
		if ch != nil && len(ch.Verses) > 0 {
			if _, ok := bc.Chapters[ch.Number]; !ok {
				bc.ChapterOrder = append(bc.ChapterOrder, ch.Number)
			}
			bc.Chapters[ch.Number] = ch
		}
	}

	startChapter := func(n int) {
		flushChapter()
		if n <= 0 {
			n = chapter + 1
		}
		chapter = n
		ch = &model.Chapter{VersionID: versionID, Collection: collection, Book: bookOrdinal, Number: n, Verses: map[int]string{}, Order: []int{}}
		lastVerse = 0
		nextVerse = 1
		pendingNumOnly = 0
	}

	startVerse := func(n int) {
		if ch == nil {
			startChapter(1)
		}
		if n < 1 {
			n = 1
		}
		if _, ok := ch.Verses[n]; !ok {
			ch.Order = append(ch.Order, n)
			ch.Verses[n] = ""
		}
		lastVerse = n
		nextVerse = 0
	}

	appendText := func(curr int, txt string) {
		if _, ok := ch.Verses[curr]; ok {
			if ch.Verses[curr] == "" {
				ch.Verses[curr] = txt
			} else {
				ch.Verses[curr] = strings.TrimSpace(ch.Verses[curr] + " " + txt)
			}
		}
	}

	lines := strings.Split(text, "\n")
	firstContent := true
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// Chapter heading.
		if m := chapterRe.FindStringSubmatch(line); m != nil {
			n := 0
			if m[2] != "" {
				n, _ = strconv.Atoi(m[2])
			} else if m[3] != "" {
				n, _ = strconv.Atoi(m[3])
			}
			if n > 0 || strings.Contains(strings.ToLower(line), "chapter") || strings.Contains(line, "章") {
				startChapter(n)
				firstContent = false
				continue
			}
		}
		// Bare verse-number line (marker format).
		if m := bareNumRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			// Header guard: a long run of bare numbers at the very top (title
			// page chapter index) should not start verses. Skip them.
			if firstContent {
				pendingNumOnly++
				if pendingNumOnly > 8 {
					// Looks like a page-header index; swallow.
					firstContent = true
					continue
				}
			}
			firstContent = false
			if ch != nil {
				nextVerse = n
			}
			continue
		}
		// Leading verse number + text.
		if m := leadingRe.FindStringSubmatch(line); m != nil && ch != nil {
			n, _ := strconv.Atoi(m[1])
			nextVerse = 0
			startVerse(n)
			appendText(n, m[2])
			firstContent = false
			continue
		}
		// Normal text line.
		firstContent = false
		if ch == nil {
			startChapter(1)
		}
		if nextVerse != 0 {
			startVerse(nextVerse)
			appendText(nextVerse, line)
			nextVerse = 0
		} else if lastVerse != 0 {
			appendText(lastVerse, line)
		} else {
			// First content: verse 1.
			startVerse(1)
			appendText(1, line)
		}
	}
	flushChapter()
	for _, c := range bc.Chapters {
		if c != nil {
			total += len(c.Verses)
		}
	}
	return bc, total
}

// Needed to avoid unused import warning for os.
