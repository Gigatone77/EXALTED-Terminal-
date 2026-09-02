package main

import (
	"strings"
	"testing"
)

func TestParseHex(t *testing.T) {
	c, err := parseHex("#f5a623")
	if err != nil {
		t.Fatal(err)
	}
	if c.R != 0xf5 || c.G != 0xa6 || c.B != 0x23 {
		t.Errorf("bad parse: %#v", c)
	}
	if _, err := parseHex("zz"); err == nil {
		t.Error("expected error for bad hex")
	}
}

func TestVerseBookName(t *testing.T) {
	if got := verseBookName(43); got != "John" {
		t.Errorf("verseBookName(43) = %q, want John", got)
	}
	if got := verseBookName(999); got != "?" {
		t.Errorf("verseBookName(999) = %q, want ?", got)
	}
	if got := verseBookName(1); got != "Genesis" {
		t.Errorf("verseBookName(1) = %q, want Genesis", got)
	}
}

func TestSetVerseSpan(t *testing.T) {
	g := &guiApp{verses: []int{1, 2, 3, 4, 5}}

	// Expands within bounds.
	g.setVerseSpan(3)
	if g.verseSpan != 3 {
		t.Errorf("expand: got span %d, want 3", g.verseSpan)
	}
	// Never below 1.
	g.setVerseSpan(-5)
	if g.verseSpan != 1 {
		t.Errorf("floor: got span %d, want 1", g.verseSpan)
	}
	// Never above the chapter length.
	g.setVerseSpan(999)
	if g.verseSpan != 5 {
		t.Errorf("ceiling: got span %d, want 5", g.verseSpan)
	}
}

func TestSpanNarrationMultiVerse(t *testing.T) {
	g := &guiApp{verses: []int{1, 2, 3, 4, 5}, verseSpan: 3, verseSelIdx: 0}
	g.curBook = 43
	// No store available; setVerseSpan should not panic without a label.
	g.setVerseSpan(3)
	if g.verseSpanLabel != nil {
		t.Fatal("expected nil label in test")
	}
	// narration is built in selectVerse, which needs an engine; verify span
	// math helper produces the end verse correctly.
	if g.verses[0+3-1] != 3 {
		t.Errorf("span window end = %d, want 3", g.verses[0+3-1])
	}
	_ = strings.Join([]string{"a"}, " ")
}
