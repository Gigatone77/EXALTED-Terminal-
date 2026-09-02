package books

// verseCounts holds the standard verse count for each of the 66 books (per the
// traditional Protestant/KJV chapter-and-verse division). Used to rank search
// results so that terms found in small books surface ahead of common hits in
// large books.
var verseCounts = [67]int{
	0,
	1533, // Genesis
	1213, // Exodus
	859,  // Leviticus
	1288, // Numbers
	959,  // Deuteronomy
	658,  // Joshua
	618,  // Judges
	85,   // Ruth
	810,  // 1 Samuel
	695,  // 2 Samuel
	816,  // 1 Kings
	719,  // 2 Kings
	942,  // 1 Chronicles
	822,  // 2 Chronicles
	280,  // Ezra
	406,  // Nehemiah
	167,  // Esther
	1070, // Job
	2461, // Psalms
	915,  // Proverbs
	222,  // Ecclesiastes
	117,  // Song of Solomon
	1292, // Isaiah
	1364, // Jeremiah
	154,  // Lamentations
	1273, // Ezekiel
	357,  // Daniel
	197,  // Hosea
	73,   // Joel
	146,  // Amos
	21,   // Obadiah
	48,   // Jonah
	105,  // Micah
	47,   // Nahum
	56,   // Habakkuk
	53,   // Zephaniah
	38,   // Haggai
	211,  // Zechariah
	55,   // Malachi
	1071, // Matthew
	678,  // Mark
	1151, // Luke
	879,  // John
	1007, // Acts
	433,  // Romans
	437,  // 1 Corinthians
	257,  // 2 Corinthians
	149,  // Galatians
	155,  // Ephesians
	104,  // Philippians
	95,   // Colossians
	89,   // 1 Thessalonians
	47,   // 2 Thessalonians
	113,  // 1 Timothy
	83,   // 2 Timothy
	46,   // Titus
	25,   // Philemon
	303,  // Hebrews
	108,  // James
	105,  // 1 Peter
	61,   // 2 Peter
	105,  // 1 John
	13,   // 2 John
	14,   // 3 John
	25,   // Jude
	404,  // Revelation
}

// VerseCount returns the canonical number of verses in a book (1-based
// ordinal), or 0 if the ordinal is out of range.
func VerseCount(ordinal int) int {
	if ordinal < 1 || ordinal >= len(verseCounts) {
		return 0
	}
	return verseCounts[ordinal]
}
