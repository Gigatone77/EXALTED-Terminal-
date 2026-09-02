package collections

import "github.com/gigatone/biblelearn/internal/books"

// egwBooks is the ordered list of works by Ellen G. White included in the
// companion tab. Chapter counts reflect the standard published editions
// (all in the public domain; sourced from the Whites' Estate / Ellen G. White
// Estate public texts). Division is left as a generic label so the shared
// books.Book type works unchanged.
var egwBooks = []books.Book{
	{1, "Steps to Christ", "SC", 13, "EGW"},
	{2, "The Desire of Ages", "DA", 87, "EGW"},
	{3, "The Great Controversy", "GC", 42, "EGW"},
	{4, "Patriarchs and Prophets", "PP", 76, "EGW"},
	{5, "Prophets and Kings", "PK", 69, "EGW"},
	{6, "The Acts of the Apostles", "AA", 42, "EGW"},
	{7, "Thoughts from the Mount of Blessing", "MB", 10, "EGW"},
	{8, "Christ's Object Lessons", "COL", 30, "EGW"},
	{9, "The Ministry of Healing", "MH", 38, "EGW"},
	{10, "Education", "Ed", 36, "EGW"},
	{11, "The Story of Redemption", "SR", 64, "EGW"},
	{12, "Early Writings", "EW", 32, "EGW"},
}

// EllenGWhite returns the Ellen G. White collection tab.
func EllenGWhite() Collection {
	return Collection{
		ID:   "egw",
		Name: "E.G. White",
		Note: "Public-domain writings of Ellen G. White.",
		Books: func() []books.Book {
			out := make([]books.Book, len(egwBooks))
			copy(out, egwBooks)
			return out
		},
	}
}

// EGWBooks returns the ordered list of Ellen G. White books.
func EGWBooks() []books.Book {
	out := make([]books.Book, len(egwBooks))
	copy(out, egwBooks)
	return out
}
