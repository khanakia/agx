package render

import (
	"errors"
	"strings"
	"testing"
)

// errWrite is what failWriter returns.
var errWrite = errors.New("disk full")

// failWriter fails every write, like a closed pipe.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errWrite }

// TestTable_Matrix: header-only, empty cells, ragged rows and unicode cells
// (tabwriter counts runes, so a multi-byte cell still aligns).
func TestTable_Matrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		header []string
		rows   [][]string
		want   string
	}{
		{"header only", []string{"A", "B"}, nil, "A  B\n"},
		{"empty cell", []string{"A", "B"}, [][]string{{"", "y"}}, "A  B\n   y\n"},
		{"unicode aligns", []string{"NAME", "X"}, [][]string{{"wörk", "1"}, {"日本", "2"}}, "NAME  X\nwörk  1\n日本    2\n"},
		{"ragged row", []string{"A", "B", "C"}, [][]string{{"x"}}, "A  B  C\nx\n"},
	} {
		var b strings.Builder
		if err := Table(&b, c.header, c.rows); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if b.String() != c.want {
			t.Errorf("%s: %q, want %q", c.name, b.String(), c.want)
		}
	}
}

// TestTable_WriteErrors: a failing writer surfaces as an error wherever the
// tabwriter first flushes (a tab-less line ends a column block).
func TestTable_WriteErrors(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		header []string
		rows   [][]string
	}{
		"header flush": {[]string{"A"}, nil},
		"row flush":    {[]string{"A", "B"}, [][]string{{"x"}}},
		"final flush":  {[]string{"A", "B"}, [][]string{{"x", "y"}}},
	} {
		if err := Table(failWriter{}, c.header, c.rows); !errors.Is(err, errWrite) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
