package codex

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSnippet: an error body is cut to one line of at most snippetMax bytes
// plus an ellipsis, and the cut never splits a UTF-8 character (a body of
// two-byte runes used to end in half a rune).
func TestSnippet(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		in   string
		want string
	}{
		"short":         {"a  b\n c", "a b c"},
		"ascii long":    {strings.Repeat("x", snippetMax+10), strings.Repeat("x", snippetMax) + "…"},
		"multibyte cut": {strings.Repeat("é", snippetMax), strings.Repeat("é", snippetMax/2) + "…"},
		"odd boundary":  {"x" + strings.Repeat("é", snippetMax), "x" + strings.Repeat("é", (snippetMax-1)/2) + "…"},
		"empty":         {"", ""},
	} {
		got := snippet([]byte(c.in))
		if got != c.want || !utf8.ValidString(got) {
			t.Errorf("%s: snippet = %q, want %q", name, got, c.want)
		}
	}
}
