// Command bibleexport emits the offline KJV text, canon, and term-frequency
// index as JSON assets for the pure-CET EXALTED Terminal mod. It parses the
// embedded public-domain USFM directly (same source the engine imports), so
// verses are clean and Jesus' words are marked as red-letter segments.
//
// Usage:
//
//	bibleexport -out <mod data dir>
//
// Writes books.json, kjv/<ordinal>.json (one per book), and terms.json.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gigatone/biblelearn/internal/books"
	"github.com/gigatone/biblelearn/internal/bundled"
	"github.com/gigatone/biblelearn/internal/collections"
)

// Footnote markers hold cross-references/translation notes that are not part
// of the scripture text. They are matched non-greedily to their \f* close.
var reFootnote = regexp.MustCompile(`(?s)\\\f\s+.*?\\f\*`)

// reCloseArg covers a close tag and its trailing asterisk (\w*, \+w*, \add*,
// \nd*, \it* ...). Cut WITHOUT consuming the following space, so words stay
// separated — must run before reArg, which would otherwise strip the \w
// opener of the close tag and leave a stray asterisk.
var reCloseArg = regexp.MustCompile(`\\(?:\+[a-zA-Z0-9]+|[a-zA-Z0-9]+)\*`)

// reArg covers open tags (\w, \+w, \add, \nd, \it ...) plus the single
// trailing space that follows them.
var reArg = regexp.MustCompile(`\\(?:\+[a-zA-Z0-9]+|[a-zA-Z0-9]+)\s?`)

var reStrong = regexp.MustCompile(`\|strong="[^"]*"\*?`)
var reSpaces = regexp.MustCompile(`\s+`)

// cleanChunk strips USFM word-level markup from one span of verse text.
// Internal whitespace is collapsed but leading/trailing space is kept — the
// glue between red and non-red segments must survive.
func cleanChunk(s string) string {
	s = reFootnote.ReplaceAllString(s, " ")
	s = reStrong.ReplaceAllString(s, "")
	s = reCloseArg.ReplaceAllString(s, "")
	s = reArg.ReplaceAllString(s, "")
	s = reSpaces.ReplaceAllString(s, " ")
	return s
}

type segOut struct {
	T string `json:"t"`
	R bool   `json:"r"`
}

var reWJ = regexp.MustCompile(`(\\wj\*?)`)

// splitRedLetter turns raw USFM verse text into segments, marking "Words of
// Jesus" (\wj ... \wj*) spans as red. Returns nil if the verse has no
// red-letter spans.
func splitRedLetter(raw string) []segOut {
	if !strings.Contains(raw, `\wj`) {
		return nil
	}
	parts := reWJ.Split(raw, -1)
	markers := reWJ.FindAllString(raw, -1)
	red := false
	var segs []segOut
	for i, p := range parts {
		if i > 0 {
			if markers[i-1] == `\wj` {
				red = true
			} else if markers[i-1] == `\wj*` {
				red = false
			}
		}
		t := strings.TrimSpace(cleanChunk(p))
		if t == "" {
			continue
		}
		if len(segs) > 0 && segs[len(segs)-1].R == red {
			segs[len(segs)-1].T += " " + t
		} else {
			segs = append(segs, segOut{T: t, R: red})
		}
	}
	if segs == nil {
		return nil
	}
	if len(segs) == 1 && !segs[0].R {
		return nil
	}
	return segs
}

type verseOut struct {
	V    int      `json:"v"`
	Text string   `json:"text,omitempty"`
	Segs []segOut `json:"segs,omitempty"`
}

func main() {
	out := flag.String("out", "", "output directory for mod data assets")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "bibleexport: -out required")
		os.Exit(2)
	}
	if err := os.MkdirAll(filepath.Join(*out, "kjv"), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "bibleexport:", err)
		os.Exit(1)
	}

	usfm, err := bundled.EmbeddedKJVUSFM()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bibleexport:", err)
		os.Exit(1)
	}

	blist := collections.ByIDSafe("bible")
	type bookOut struct {
		N        int    `json:"n"`
		Name     string `json:"name"`
		Short    string `json:"short"`
		Chapters int    `json:"chapters"`
		Verses   int    `json:"verses"`
	}
	var canon []bookOut
	for _, b := range blist {
		canon = append(canon, bookOut{b.Ordinal, b.Name, b.Short, b.Chapters, books.VerseCount(b.Ordinal)})
	}
	if err := writeJSON(filepath.Join(*out, "books.json"), canon); err != nil {
		fmt.Fprintln(os.Stderr, "bibleexport:", err)
		os.Exit(1)
	}

	booksData := map[int]map[string][]verseOut{}
	termFreq := map[string]int{}
	totalVerses := 0

	idRegex := regexp.MustCompile(`\\id\s+([^\s]+)`)
	cRegex := regexp.MustCompile(`(^|\s)\\c\s+([0-9]+)`)
	vRegex := regexp.MustCompile(`(^|\s)\\v\s+([0-9]+)`)
	idToOrd := map[string]int{
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

	curBook := 0
	curChapter := 0
	curVerses := map[int]string{}
	curOrder := []int{}
	haveChapter := false

	addChapter := func() {
		if curBook == 0 || !haveChapter || len(curOrder) == 0 {
			return
		}
		if booksData[curBook] == nil {
			booksData[curBook] = map[string][]verseOut{}
		}
		ch := strconv.Itoa(curChapter)
		for _, vn := range curOrder {
			raw := curVerses[vn]
			segs := splitRedLetter(raw)
			var vo verseOut
			if segs != nil {
				vo.V = vn
				vo.Text = segsText(segs)
				vo.Segs = segs
			} else {
				vo.V = vn
				vo.Text = strings.TrimSpace(cleanChunk(raw))
			}
			booksData[curBook][ch] = append(booksData[curBook][ch], vo)
			totalVerses++
			for _, w := range regexp.MustCompile(`[a-z']+`).FindAllString(strings.ToLower(vo.Text), -1) {
				termFreq[w]++
			}
		}
	}

	sc := bufio.NewScanner(strings.NewReader(usfm))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)

	// A line may carry a verse AND an inline \id for the next book (ebible KJV
	// appends "\id XXX" at the very end of the previous book's last verse).
	// The verse portion must be processed first, then the book switch.
	var handleLine func(line string)
	skip := false
	handleLine = func(line string) {
		trim := strings.TrimSpace(line)
		if idm := idRegex.FindStringSubmatch(line); idm != nil {
			idx := strings.Index(line, idm[0])
			head := ""
			if idx > 0 {
				head = line[:idx]
			}
			if ord := idToOrd[strings.ToUpper(idm[1])]; ord != 0 {
				resid := ""
				if idx >= 0 {
					resid = line[idx+len(idm[0]):]
					if m := idRegex.FindStringSubmatchIndex(resid); m != nil {
						resid = ""
					}
				}
				handleLine(head)
				if curVerses != nil && len(curOrder) > 0 && strings.Contains(resid, `\`) {
					last := curOrder[len(curOrder)-1]
					curVerses[last] = strings.TrimSpace(curVerses[last] + " " + resid)
				}
				addChapter()
				curBook = ord
				haveChapter = false
				curVerses = map[int]string{}
				curOrder = nil
				curChapter = 0
				skip = false
				return
			}
			// A non-canonical book (\id TOB, ...) begins inline on this line —
			// the verse head still belongs to the current book. Flush it.
			handleLine(head)
			addChapter()
			skip = true
			haveChapter = false
			curVerses = nil
			curOrder = nil
			curChapter = 0
			return
		}
		if skip {
			return
		}
		if cm := cRegex.FindStringSubmatch(line); cm != nil {
			addChapter()
			n, _ := strconv.Atoi(cm[2])
			curChapter = n
			curVerses = map[int]string{}
			curOrder = nil
			haveChapter = true
			return
		}
		if vm := vRegex.FindStringSubmatch(line); vm != nil {
			if !haveChapter || curBook == 0 {
				return
			}
			n, _ := strconv.Atoi(vm[2])
			idx := strings.Index(line, vm[2])
			rest := ""
			if idx >= 0 {
				k := idx + len(vm[2])
				if k < len(line) {
					rest = line[k:]
				}
			}
			curVerses[n] = rest
			curOrder = append(curOrder, n)
			return
		}
		if haveChapter && len(curOrder) > 0 {
			cont := strings.TrimSpace(trim)
			if cont != "" {
				last := curOrder[len(curOrder)-1]
				curVerses[last] = strings.TrimSpace(curVerses[last] + " " + cont)
			}
		}
	}

	for sc.Scan() {
		handleLine(sc.Text())
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "bibleexport:", err)
		os.Exit(1)
	}
	addChapter()

	for ord, chs := range booksData {
		if err := writeJSON(filepath.Join(*out, "kjv", fmt.Sprintf("%d.json", ord)), chs); err != nil {
			fmt.Fprintln(os.Stderr, "bibleexport:", err)
			os.Exit(1)
		}
	}

	type termOut struct {
		Term  string `json:"term"`
		Count int    `json:"count"`
	}
	var terms []termOut
	for t, c := range termFreq {
		terms = append(terms, termOut{t, c})
	}
	sort.Slice(terms, func(i, j int) bool {
		if terms[i].Count != terms[j].Count {
			return terms[i].Count > terms[j].Count
		}
		return terms[i].Term < terms[j].Term
	})
	if err := writeJSON(filepath.Join(*out, "terms.json"), terms); err != nil {
		fmt.Fprintln(os.Stderr, "bibleexport:", err)
		os.Exit(1)
	}

	red := 0
	for _, chs := range booksData {
		for _, vs := range chs {
			for _, vo := range vs {
				if vo.Segs != nil {
					red++
				}
			}
		}
	}
	fmt.Printf("bibleexport: %d books, %d verses, %d red-letter verses, %d unique terms -> %s\n",
		len(canon), totalVerses, red, len(terms), *out)
}

func segsText(segs []segOut) string {
	var parts []string
	for _, s := range segs {
		parts = append(parts, s.T)
	}
	return strings.Join(parts, " ")
}

func writeJSON(path string, v any) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmpw, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	name := tmpw.Name()
	defer os.Remove(name)
	if _, err := tmpw.Write(buf); err != nil {
		tmpw.Close()
		return err
	}
	if err := tmpw.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
