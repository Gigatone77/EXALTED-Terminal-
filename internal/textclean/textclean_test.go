package textclean

import "testing"

func TestHTML(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`And the tree <i>that</i> yields fruit`, `And the tree that yields fruit`},
		{`whose seed <i>is</i> in itself`, `whose seed is in itself`},
		{`And God saw that <i>it</i> <i>was</i> good.`, `And God saw that it was good.`},
		{`a &amp; b`, `a & b`},
		{`x &lt; y`, `x < y`},
		{`plain text`, `plain text`},
		{`two  spaces  collapse`, `two spaces collapse`},
		{`lead <i>tag</i>`, `lead tag`},
	}
	for _, c := range cases {
		if got := HTML(c.in); got != c.want {
			t.Errorf("HTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
