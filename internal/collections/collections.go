// Package collections defines the library of scripture and related writings
// that can be browsed. The app is designed around multiple collections shown as
// tabs: the canonical Bible plus the public-domain writings of Ellen G. White
// (a companion "spirit of prophecy" library). Each collection has its own
// ordered list of books and chapter counts.
package collections

import "github.com/gigatone/biblelearn/internal/books"

// Collection describes one browsable library.
type Collection struct {
	// ID is the canonical identifier, e.g. "bible" or "egw".
	ID string
	// Name is the display name, e.g. "Bible" or "Ellen G. White".
	Name string
	// Source note shown in UIs.
	Note string
	// Books returns the ordered books in the collection.
	Books func() []books.Book
}

// Bible returns the canonical Bible collection.
func Bible() Collection {
	return Collection{
		ID:    "bible",
		Name:  "Bible",
		Note:  "The Holy Bible, 66 books.",
		Books: books.Books,
	}
}

// All returns every known collection in tab order.
func All() []Collection {
	return []Collection{Bible(), EllenGWhite()}
}

// ByID returns a collection by its id, if known.
func ByID(id string) (Collection, bool) {
	for _, c := range All() {
		if c.ID == id {
			return c, true
		}
	}
	return Collection{}, false
}

// ByIDSafe returns the book list for a collection, defaulting to the Bible.
func ByIDSafe(id string) []books.Book {
	c, ok := ByID(id)
	if !ok {
		c = Bible()
	}
	return c.Books()
}

// Resolve looks up a book by name/short-name/unique-prefix within a collection.
// Returns the collection book with a 1-based ordinal.
func Resolve(collectionID, s string) (books.Book, bool) {
	c, ok := ByID(collectionID)
	if !ok {
		c = Bible()
	}
	list := c.Books()
	if s == "" {
		return books.Book{}, false
	}
	// Exact localized alias match first.
	if ord, ok := localizedOrdinal(collectionID, s); ok && ord >= 1 && ord <= len(list) {
		return list[ord-1], true
	}
	norm := normalize(s)
	var matches []books.Book
	for _, b := range list {
		if normalize(b.Name) == norm || normalize(b.Short) == norm {
			return b, true
		}
	}
	for _, b := range list {
		bn := normalize(b.Name)
		if len(norm) >= 2 && len(bn) >= len(norm) && bn[:len(norm)] == norm {
			matches = append(matches, b)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return books.Book{}, false
}

func normalize(s string) string {
	lower := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != ' ' && c != '.' {
			lower = append(lower, c)
		}
	}
	return string(lower)
}
