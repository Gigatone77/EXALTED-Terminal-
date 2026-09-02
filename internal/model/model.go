// Package model defines the core domain types shared across the app: version
// metadata, verse references, and the canonical schemes for addressing and
// storing scripture.
package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gigatone/biblelearn/internal/collections"
)

// Version identifies a Bible translation and where its data comes from.
type Version struct {
	// ID is the canonical identifier used in storage, e.g. "KJV" or "NKJV".
	ID string
	// Name is the human-readable name, e.g. "King James Version".
	Name string
	// Lang is the BCP-47 language tag, e.g. "en".
	Lang string
	// Collection scopes this version to a tab (e.g. "bible" or "egw").
	Collection string
	// Source describes where the text was obtained (e.g. "Internet Archive",
	// "user import", "bundled").
	Source string
	// SourceURL records the origin URL for provenance (optional).
	SourceURL string
	// Builtin is true for versions bundled with the binary (e.g. KJV).
	Builtin bool
	// Enabled marks whether the version is currently available for use.
	Enabled bool
}

// Ref is a fully-qualified reference in canonical (1-based) coordinates.
type Ref struct {
	// VersionID selects the translation.
	VersionID string `json:"version"`
	// Collection scopes the reference (default "bible").
	Collection string `json:"collection,omitempty"`
	// Book is the 1-based ordinal within the collection.
	Book int `json:"book"`
	// Chapter is the 1-based chapter number.
	Chapter int `json:"chapter"`
	// Verse is the 1-based verse/paragraph number.
	Verse int `json:"verse"`
}

// String renders a human-readable reference, e.g. "John 3:16".
func (r Ref) String() string {
	label := fmt.Sprintf("%s %d:%d", r.BookName(), r.Chapter, r.Verse)
	if r.VersionID != "" {
		label += " (" + r.VersionID + ")"
	}
	return label
}

// BookName returns the canonical name for the reference's book ordinal within
// its collection (defaulting to the Bible canon).
func (r Ref) BookName() string {
	col := r.Collection
	if col == "" {
		col = "bible"
	}
	if c, ok := collections.ByID(col); ok {
		bs := c.Books()
		if r.Book >= 1 && r.Book <= len(bs) {
			return bs[r.Book-1].Name
		}
	}
	return "?"
}

// Verse is a single unit of scripture text within a version.
type Verse struct {
	// Row is the storage key (version-normalized seq). Not user-facing.
	Row int64 `json:"-"`
	// Ref identifies where the verse lives.
	Ref Ref `json:"ref"`
	// Text is the verse body without any verse numeral.
	Text string `json:"text"`
}

// Chapter is a full chapter's worth of verses for one version.
type Chapter struct {
	VersionID  string         `json:"version"`
	Collection string         `json:"collection,omitempty"`
	Book       int            `json:"book"`
	Number     int            `json:"chapter"`
	Verses     map[int]string `json:"verses"` // verse# -> text
	Order      []int          `json:"order"`  // stable verse ordering
}

// BookContent is the complete text of one book (all its chapters) for one
// version in one collection. This is the unit stored per archive file.
type BookContent struct {
	VersionID    string           `json:"version"`
	Collection   string           `json:"collection,omitempty"`
	Book         int              `json:"book"`
	BookName     string           `json:"book_name"`
	Chapters     map[int]*Chapter `json:"chapters"`      // chapter# -> chapter
	ChapterOrder []int            `json:"chapter_order"` // stable chapter ordering
}

// NewBookContent returns an empty container for a book.
func NewBookContent(versionID, collection string, bookOrdinal int) *BookContent {
	return &BookContent{
		VersionID:  versionID,
		Collection: collection,
		Book:       bookOrdinal,
		Chapters:   map[int]*Chapter{},
	}
}

// ParseRef parses a reference string like "John 3:16", "Gen 1:1-2",
// "Psalm 23", or "Ps 23:1, 5-7" into canonical coordinates in the Bible canon.
// The version is resolved separately by the caller. Verbatim verse lists and
// ranges are flattened into the first verse of the final range for addressing.
func ParseRef(s string) (Ref, bool) {
	return ParseRefInCollection("bible", s)
}

// ParseRefInCollection parses a reference within the given collection, so books
// are resolved by that collection's book list.
func ParseRefInCollection(collection, s string) (Ref, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Ref{}, false
	}
	// Book name is everything before the first digit that starts a chapter.
	idx := -1
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return Ref{}, false
	}
	bookName := strings.TrimSpace(s[:idx])
	rest := s[idx:]
	b, ok := collections.Resolve(collection, bookName)
	if !ok {
		return Ref{}, false
	}
	// Remove any commas (multiple ranges) -> keep first for addressing.
	rest = strings.SplitN(rest, ",", 2)[0]
	// Split chapter and verse by ':'.
	var chapStr, verseStr string
	if c := strings.Index(rest, ":"); c >= 0 {
		chapStr = rest[:c]
		verseStr = rest[c+1:]
	} else {
		chapStr = rest
	}
	chapStr = strings.TrimSpace(chapStr)
	// Take first number from verse range (e.g. "16-18" -> 16).
	verseStr = strings.TrimSpace(verseStr)
	if d := strings.Index(verseStr, "-"); d >= 0 {
		verseStr = strings.TrimSpace(verseStr[:d])
	}
	chap, err := strconv.Atoi(chapStr)
	if err != nil {
		return Ref{}, false
	}
	verse := 0
	if verseStr != "" {
		verse, err = strconv.Atoi(verseStr)
		if err != nil {
			return Ref{}, false
		}
	}
	if chap < 1 || chap > b.Chapters {
		return Ref{}, false
	}
	if verse < 0 || verse > 200 { // generous cap; verse 0 means whole chapter
		return Ref{}, false
	}
	return Ref{Collection: collection, Book: b.Ordinal, Chapter: chap, Verse: verse}, true
}

// ParseRefFull parses "Book C:V" plus an optional version prefix like
// "NKJV John 3:16" or "John 3:16 (NKJV)".
func ParseRefFull(s string) (version string, ref Ref, ok bool) {
	s = strings.TrimSpace(s)
	// A version token is a leading uppercase-ish acronym, optionall followed by
	// the reference. Try patterns: "NKJV John 3:16" and "John 3:16 (NKJV)".
	lower := strings.ToLower(s)
	if i := strings.LastIndex(lower, "("); i >= 0 && strings.HasSuffix(lower, ")") {
		inner := strings.TrimSpace(s[i+1 : len(s)-1])
		rest := strings.TrimSpace(s[:i])
		r, ok := ParseRef(rest)
		return inner, r, ok
	}
	fields := strings.Fields(s)
	if len(fields) >= 2 {
		// Leading all-caps/known acronym token.
		if isLikelyVersionToken(fields[0]) {
			r, ok := ParseRef(strings.Join(fields[1:], " "))
			return strings.ToUpper(fields[0]), r, ok
		}
	}
	r, ok := ParseRef(s)
	return "", r, ok
}

func isLikelyVersionToken(tok string) bool {
	up := strings.ToUpper(tok)
	// Only treat a token as a version if it looks like an acronym of 2-6
	// uppercase letters and the remainder parses as a reference.
	if len(up) < 2 || len(up) > 6 {
		return false
	}
	for _, c := range up {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}
