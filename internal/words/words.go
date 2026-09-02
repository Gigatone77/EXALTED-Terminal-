// Package words tokenizes Bible text into searchable terms and identifies
// common words for the search-box suggestion index. It supports the latin
// scripts used by the built-in translations (English, Spanish, Portuguese,
// Japanese full-width forms) and normalizes case and punctuation.
package words

import (
	"strings"
	"unicode"
)

// Token is a single normalized term occurrence.
type Token struct {
	// Term is the lowercased folded surface form.
	Term string
	// FreqWeight is a simple per-occurrence weight (1 normally).
	FreqWeight int
}

// CommonEnglish lists frequent function words that add little discriminative
// value in suggestions. Kept small; the suggestion index is dominated by
// real content words due to frequency weighting, but dropping these avoids
// "the/and/of" dominating autocomplete.
var CommonEnglish = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "of": true, "to": true,
	"in": true, "for": true, "is": true, "on": true, "that": true, "with": true,
	"was": true, "he": true, "his": true, "him": true, "they": true, "their": true,
	"it": true, "you": true, "your": true, "i": true, "we": true, "not": true,
	"shall": true, "will": true, "be": true, "all": true, "as": true, "at": true,
	"by": true, "but": true, "from": true, "have": true, "has": true, "had": true,
	"this": true, "these": true, "those": true, "which": true, "who": true,
	"whom": true, "unto": true, "there": true, "were": true,
	"are": true, "am": true, "do": true, "did": true, "so": true, "if": true,
	"or": true, "up": true, "down": true, "out": true, "upon": true, "my": true,
	"mine": true, "thy": true, "thou": true, "thee": true, "thine": true,
	"our": true, "us": true, "them": true, "no": true, "ye": true, "every": true,
	"when": true, "then": true, "than": true, "also": true, "yet": true,
}

// IsCommon reports whether a term is a common stopword (English).
func IsCommon(term string) bool { return CommonEnglish[term] }

// Tokenize splits text into normalized term tokens. Non-letter characters
// delimit words; diacritics are preserved (so "amó" and "amor" stay distinct
// in Spanish) but case is folded.
func Tokenize(text string) []Token {
	var tokens []Token
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		t := cur.String()
		cur.Reset()
		if len(t) >= 2 && hasLetter(t) {
			tokens = append(tokens, Token{Term: t})
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || isApostrophe(r) {
			// Fold to lowercase (handles both ASCII and Unicode).
			cur.WriteRune(unicode.ToLower(r))
		} else if unicode.IsDigit(r) {
			// keep numbers inside words like "1cor" or standalone digits out
			// of suggestions by treating them as a separator unless within a
			// word (e.g. "3:16" -> skip).
			if cur.Len() > 0 {
				// allow digits to continue a word like "2samuel"
				cur.WriteRune(r)
			}
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

// TokenizeDistinct returns the unique terms in text as a set.
func TokenizeDistinct(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range Tokenize(text) {
		if !seen[t.Term] {
			seen[t.Term] = true
			out = append(out, t.Term)
		}
	}
	return out
}

func isApostrophe(r rune) bool {
	return r == '\'' || r == 0x2019 || r == 0x02BC
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
