// Package books defines the canonical Protestant canon metadata: the 66
// books of the Bible, their ordering, chapter counts, traditional divisions,
// and common abbreviations used for citation and lookup.
package books

// Division is a high-level grouping of books (Law, History, etc.).
type Division string

const (
	Law        Division = "Law"
	History    Division = "History"
	Wisdom     Division = "Wisdom"
	MajorProph Division = "MajorProphets"
	MinorProph Division = "MinorProphets"
	Gospels    Division = "Gospels"
	Acts       Division = "Acts"
	Epistles   Division = "Epistles"
	Revelation Division = "Revelation"
)

// Book is the static metadata for a book of the Bible.
type Book struct {
	// Ordinal is the 1-based position in the canon (1 = Genesis).
	Ordinal int
	// Name is the canonical full name, e.g. "Genesis".
	Name string
	// Short is the conventional abbreviated form, e.g. "Gen".
	Short string
	// Chapters is the number of chapters in the book.
	Chapters int
	// Division groups the book (Law, History, ...).
	Division Division
}

// canon is the ordered list of the 66 books. Chapter counts follow the
// traditional Protestant Protestant textual tradition.
var canon = []Book{
	{1, "Genesis", "Gen", 50, Law},
	{2, "Exodus", "Ex", 40, Law},
	{3, "Leviticus", "Lev", 27, Law},
	{4, "Numbers", "Num", 36, Law},
	{5, "Deuteronomy", "Deut", 34, Law},
	{6, "Joshua", "Josh", 24, History},
	{7, "Judges", "Judg", 21, History},
	{8, "Ruth", "Ruth", 4, History},
	{9, "1 Samuel", "1Sam", 31, History},
	{10, "2 Samuel", "2Sam", 24, History},
	{11, "1 Kings", "1Kgs", 22, History},
	{12, "2 Kings", "2Kgs", 25, History},
	{13, "1 Chronicles", "1Chr", 29, History},
	{14, "2 Chronicles", "2Chr", 36, History},
	{15, "Ezra", "Ezra", 10, History},
	{16, "Nehemiah", "Neh", 13, History},
	{17, "Esther", "Esth", 10, History},
	{18, "Job", "Job", 42, Wisdom},
	{19, "Psalms", "Ps", 150, Wisdom},
	{20, "Proverbs", "Prov", 31, Wisdom},
	{21, "Ecclesiastes", "Eccl", 12, Wisdom},
	{22, "Song of Solomon", "Song", 8, Wisdom},
	{23, "Isaiah", "Isa", 66, MajorProph},
	{24, "Jeremiah", "Jer", 52, MajorProph},
	{25, "Lamentations", "Lam", 5, MajorProph},
	{26, "Ezekiel", "Ezek", 48, MajorProph},
	{27, "Daniel", "Dan", 12, MajorProph},
	{28, "Hosea", "Hos", 14, MinorProph},
	{29, "Joel", "Joel", 3, MinorProph},
	{30, "Amos", "Amos", 9, MinorProph},
	{31, "Obadiah", "Obad", 1, MinorProph},
	{32, "Jonah", "Jonah", 4, MinorProph},
	{33, "Micah", "Mic", 7, MinorProph},
	{34, "Nahum", "Nah", 3, MinorProph},
	{35, "Habakkuk", "Hab", 3, MinorProph},
	{36, "Zephaniah", "Zeph", 3, MinorProph},
	{37, "Haggai", "Hag", 2, MinorProph},
	{38, "Zechariah", "Zech", 14, MinorProph},
	{39, "Malachi", "Mal", 4, MinorProph},
	{40, "Matthew", "Matt", 28, Gospels},
	{41, "Mark", "Mark", 16, Gospels},
	{42, "Luke", "Luke", 24, Gospels},
	{43, "John", "John", 21, Gospels},
	{44, "Acts", "Acts", 28, Acts},
	{45, "Romans", "Rom", 16, Epistles},
	{46, "1 Corinthians", "1Cor", 16, Epistles},
	{47, "2 Corinthians", "2Cor", 13, Epistles},
	{48, "Galatians", "Gal", 6, Epistles},
	{49, "Ephesians", "Eph", 6, Epistles},
	{50, "Philippians", "Phil", 4, Epistles},
	{51, "Colossians", "Col", 4, Epistles},
	{52, "1 Thessalonians", "1Thess", 5, Epistles},
	{53, "2 Thessalonians", "2Thess", 3, Epistles},
	{54, "1 Timothy", "1Tim", 6, Epistles},
	{55, "2 Timothy", "2Tim", 4, Epistles},
	{56, "Titus", "Titus", 3, Epistles},
	{57, "Philemon", "Philem", 1, Epistles},
	{58, "Hebrews", "Heb", 13, Epistles},
	{59, "James", "Jas", 5, Epistles},
	{60, "1 Peter", "1Pet", 5, Epistles},
	{61, "2 Peter", "2Pet", 3, Epistles},
	{62, "1 John", "1John", 5, Epistles},
	{63, "2 John", "2John", 1, Epistles},
	{64, "3 John", "3John", 1, Epistles},
	{65, "Jude", "Jude", 1, Epistles},
	{66, "Revelation", "Rev", 22, Revelation},
}

// Books returns the full ordered canon.
func Books() []Book {
	out := make([]Book, len(canon))
	copy(out, canon)
	return out
}

// TotalChapters returns the total number of chapters across the canon.
func TotalChapters() int {
	n := 0
	for _, b := range canon {
		n += b.Chapters
	}
	return n
}

// ByOrdinal returns a copy of the book at the given 1-based ordinal, or false.
func ByOrdinal(o int) (Book, bool) {
	if o < 1 || o > len(canon) {
		return Book{}, false
	}
	return canon[o-1], true
}

// Resolve looks up a book by full name, short name, or a unique prefix
// (case-insensitive). Numeric forms like "1Tim" and "1 Tim" are supported.
func Resolve(s string) (Book, bool) {
	if s == "" {
		return Book{}, false
	}
	// Normalize: trim spaces, lowercase.
	norm := normalize(s)
	for _, b := range canon {
		if normalize(b.Name) == norm || normalize(b.Short) == norm {
			return b, true
		}
	}
	// Prefix match (shortest unique prefix).
	var matches []Book
	for _, b := range canon {
		bn := normalize(b.Name)
		if len(norm) >= 2 && len(bn) >= len(norm) && bn[:len(norm)] == norm {
			matches = append(matches, b)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return Book{}, false
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

// ResolveOrdinal returns the ordinal of a book given an exact full or short
// name (case-insensitive), without prefix matching. It returns 0 for
// unrecognized input. This is used where a string must match a book heading
// exactly (e.g. detecting a new book while importing a whole PDF).
func ResolveOrdinal(s string) int {
	norm := normalize(s)
	for _, b := range canon {
		if normalize(b.Name) == norm || normalize(b.Short) == norm {
			return b.Ordinal
		}
	}
	return 0
}
