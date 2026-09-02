package main

import "testing"

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