// Package bibleimport turns external Bible text (USFM, plain numbered text,
// or text fetched from the Internet Archive) into the store's version archive.
package bibleimport

import (
	"bufio"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/gigatone/biblelearn/internal/books"
	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/store"
)

// Progress reports incremental import progress.
type Progress struct {
	Phase       string
	Book        string
	Chapter     int
	VersesAdded int
}

// IsUSFM reports whether content appears to be USFM-formatted (has \id or \v
// markers on their own) rather than plain text.
func IsUSFM(content string) bool {
	return idRegex.MatchString(content) || strings.Contains(content, `\v `) || strings.Contains(content, `\c `)
}

// ImportOptions controls how source text is interpreted.
type ImportOptions struct {
	// Collection scopes the imported books (default "bible").
	Collection string
	// Progress, if non-nil, is invoked as import proceeds.
	Progress func(Progress)
}

var (
	idRegex   = regexp.MustCompile(`(?m)\\id\s+([^\s]+)`)
	cRegex    = regexp.MustCompile(`(^|\s)\\c\s+([0-9]+)`)
	vRegex    = regexp.MustCompile(`(^|\s)\\v\s+([0-9]+)`)
	footRegex = regexp.MustCompile(`\\f[^\\]*\\f\*`)
	otherReg  = regexp.MustCompile(`\\([a-zA-Z0-9]+|\*)\s*`)
	spaces    = regexp.MustCompile(`\s+`)
)

// ImportUSFMText ingests USFM content and writes the parsed text into the
// version archive. Returns the number of verses imported. Every book present
// in the file is stored separately and its FTS index refreshed.
func ImportUSFMText(s *store.Store, version string, content string, opt ImportOptions) (int, error) {
	coll := opt.Collection
	if coll == "" {
		coll = "bible"
	}
	// content -> open books.
	bookContent := map[int]*model.BookContent{}
	openBook := func(ord int) *model.BookContent {
		bc, ok := bookContent[ord]
		if !ok {
			bc = model.NewBookContent(version, coll, ord)
			bookContent[ord] = bc
		}
		return bc
	}

	curBook := 0
	curChapter := 0
	var curVerses map[int]string
	var curOrder []int
	haveChapter := false

	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		// \id markers may appear on their own line or embedded at the end of a
		// verse line after concatenation; match them anywhere in the line.
		if m := idRegex.FindStringSubmatch(trim); m != nil {
			if b := idToBook(m[1]); b != 0 {
				// Commit the previously open chapter to its book before moving on.
				if curBook != 0 {
					flushChapter(curBook, openBook(curBook), &model.Chapter{VersionID: version, Collection: coll, Book: curBook, Number: curChapter, Verses: curVerses, Order: curOrder}, haveChapter)
				}
				curBook = b
				haveChapter = false
				curVerses = nil
				curOrder = nil
				curChapter = 0
			}
			continue
		}
		if m := cRegex.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			// Commit the previous chapter before starting a new one.
			if curBook != 0 {
				flushChapter(curBook, openBook(curBook), &model.Chapter{VersionID: version, Collection: coll, Book: curBook, Number: curChapter, Verses: curVerses, Order: curOrder}, haveChapter)
			}
			curChapter = n
			curVerses = map[int]string{}
			curOrder = nil
			haveChapter = true
			continue
		}
		if m := vRegex.FindStringSubmatch(line); m != nil {
			if !haveChapter || curVerses == nil {
				continue
			}
			n, _ := strconv.Atoi(m[2])
			// Extract text to the right of the marker.
			idx := strings.Index(line, m[2])
			rest := ""
			if idx >= 0 {
				k := idx + len(m[2])
				if k < len(line) {
					rest = line[k:]
				}
			}
			text := cleanVerse(rest)
			curVerses[n] = text
			curOrder = append(curOrder, n)
			continue
		}
		// Continuation line: append to the current verse.
		if haveChapter && curVerses != nil && len(curOrder) > 0 {
			last := curOrder[len(curOrder)-1]
			cont := cleanVerse(trim)
			if cont != "" {
				curVerses[last] = strings.TrimSpace(curVerses[last] + " " + cont)
			}
		}
	}

	// Flush the last chapter to its book.
	if curBook != 0 {
		flushChapter(curBook, openBook(curBook), &model.Chapter{VersionID: version, Collection: coll, Book: curBook, Number: curChapter, Verses: curVerses, Order: curOrder}, haveChapter)
	}

	total := 0
	for ord, bc := range bookContent {
		if err := writeBook(s, version, ord, bc, opt); err != nil {
			return total, err
		}
		total += countBookVerses(bc)
	}
	return total, nil
}

func addChapter(bc *model.BookContent, ch *model.Chapter) {
	if _, ok := bc.Chapters[ch.Number]; !ok {
		bc.ChapterOrder = append(bc.ChapterOrder, ch.Number)
	}
	bc.Chapters[ch.Number] = ch
}

// flushChapter commits a chapter into a book if it has any verses.
func flushChapter(book int, bc *model.BookContent, ch *model.Chapter, haveChapter bool) {
	if book == 0 || !haveChapter || ch == nil || len(ch.Verses) == 0 {
		return
	}
	if _, ok := bc.Chapters[ch.Number]; !ok {
		bc.ChapterOrder = append(bc.ChapterOrder, ch.Number)
	}
	bc.Chapters[ch.Number] = ch
}

func countBookVerses(bc *model.BookContent) int {
	n := 0
	for _, ch := range bc.Chapters {
		n += len(ch.Verses)
	}
	return n
}

func writeBook(s *store.Store, version string, ord int, bc *model.BookContent, opt ImportOptions) error {
	coll := opt.Collection
	if coll == "" {
		coll = "bible"
	}
	if err := s.PutBook(version, coll, ord, bc); err != nil {
		return err
	}
	if err := s.IndexBook(version, coll, ord); err != nil {
		return err
	}
	if opt.Progress != nil {
		b, _ := books.ByOrdinal(ord)
		opt.Progress(Progress{Phase: "book", Book: b.Name, VersesAdded: countBookVerses(bc)})
	}
	return nil
}

func cleanVerse(text string) string {
	text = otherReg.ReplaceAllString(text, "")
	text = footRegex.ReplaceAllString(text, " ")
	text = spaces.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// ImportPlainText imports a plain-text Bible in the common form:
//
//	Book chapter:verse text
//	e.g. "Genesis 1:1 In the beginning..."
//	or    "> Gen 1:1 In the beginning"
//	or    "1:1 In the beginning" under a "## Genesis" heading.
func ImportPlainText(s *store.Store, version string, content string, opt ImportOptions) (int, error) {
	coll := opt.Collection
	if coll == "" {
		coll = "bible"
	}
	bookContent := map[int]*model.BookContent{}
	openBook := func(ord int) *model.BookContent {
		bc, ok := bookContent[ord]
		if !ok {
			bc = model.NewBookContent(version, coll, ord)
			bookContent[ord] = bc
		}
		return bc
	}
	curBook := 0
	curChapter := 0
	haveChapter := false
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	total := 0

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			clean := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
			if b, ok := books.Resolve(clean); ok {
				curBook = b.Ordinal
				haveChapter = false
				continue
			}
		}
		if b, ok := books.Resolve(line); ok && !isVerseLine(line) {
			curBook = b.Ordinal
			haveChapter = false
			continue
		}
		if curBook == 0 {
			continue
		}
		// Handle "chapter:verse text" or "verse text".
		vn, text, chapter, ok := parseLine(line)
		if !ok {
			continue
		}
		if chapter != 0 {
			curChapter = chapter
			haveChapter = true
		}
		if !haveChapter || vn == 0 {
			continue
		}
		bc := openBook(curBook)
		ch, okCh := bc.Chapters[curChapter]
		if !okCh {
			ch = &model.Chapter{VersionID: version, Collection: coll, Book: curBook, Number: curChapter, Verses: map[int]string{}, Order: []int{}}
			bc.Chapters[curChapter] = ch
			bc.ChapterOrder = append(bc.ChapterOrder, curChapter)
		}
		if _, dup := ch.Verses[vn]; !dup {
			ch.Order = append(ch.Order, vn)
		}
		ch.Verses[vn] = strings.TrimSpace(text)
		total++
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return total, err
	}
	for ord, bc := range bookContent {
		if err := writeBook(s, version, ord, bc, opt); err != nil {
			return total, err
		}
	}
	return total, nil
}

func isVerseLine(line string) bool {
	return parseRefLine(line)
}

var reFullRef = regexp.MustCompile(`^([0-9]+):([0-9]+)\s+(.*)$`)

// parseLine recognizes "C:V text", "V text", or "Book C:V text".
func parseLine(line string) (verse int, text string, chapter int, ok bool) {
	if m := reFullRef.FindStringSubmatch(line); m != nil {
		c, _ := strconv.Atoi(m[1])
		v, _ := strconv.Atoi(m[2])
		return v, strings.TrimSpace(m[3]), c, true
	}
	return 0, "", 0, false
}

func parseRefLine(line string) bool {
	return reFullRef.MatchString(line)
}

// idToBook maps USFM book codes to ordinals.
func idToBook(code string) int {
	code = strings.ToUpper(strings.TrimSpace(code))
	m := map[string]int{
		"GEN": 1, "EXO": 2, "LEV": 3, "NUM": 4, "DEU": 5, "JOS": 6, "JDG": 7,
		"RUT": 8, "1SA": 9, "2SA": 10, "1KI": 11, "2KI": 12, "1CH": 13, "2CH": 14,
		"EZR": 15, "NEH": 16, "EST": 17, "JOB": 18, "PSA": 19, "PRO": 20,
		"ECC": 21, "SNG": 22, "ISA": 23, "JER": 24, "LAM": 25, "EZK": 26,
		"DAN": 27, "HOS": 28, "JOL": 29, "AMO": 30, "OBA": 31, "JON": 32,
		"MIC": 33, "NAM": 34, "HAB": 35, "ZEP": 36, "HAG": 37, "ZEC": 38,
		"MAL": 39, "MAT": 40, "MRK": 41, "LUK": 42, "JHN": 43, "ACT": 44,
		"ROM": 45, "1CO": 46, "2CO": 47, "GAL": 48, "EPH": 49, "PHP": 50,
		"COL": 51, "1TH": 52, "2TH": 53, "1TI": 54, "2TI": 55, "TIT": 56,
		"PHM": 57, "HEB": 58, "JAS": 59, "1PE": 60, "2PE": 61, "1JN": 62,
		"2JN": 63, "3JN": 64, "JUD": 65, "REV": 66,
	}
	return m[code]
}
