// Package textclean normalizes scripture text for display: it removes
// common HTML tags (e.g. the <i>…</i> italics markup used by the GetBible
// NKJV source) so passages render as plain, readable text.
package textclean

import (
	"strings"
)

// HTML removes HTML tags (and their content-formatting) from s, collapsing
// entities like &amp; &lt; &gt; &nbsp; and trimming whitespace.
func HTML(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	n := len(s)
	for i < n {
		c := s[i]
		if c == '<' {
			// Skip to the closing '>' of the tag.
			j := i + 1
			for j < n && s[j] != '>' {
				j++
			}
			if j < n {
				i = j + 1
				continue
			}
			i = n
			break
		}
		b.WriteByte(c)
		i++
	}
	out := b.String()
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&quot;", "\"")
	out = strings.ReplaceAll(out, "&#39;", "'")
	out = strings.ReplaceAll(out, "&nbsp;", " ")
	out = strings.Join(strings.Fields(out), " ")
	return out
}
