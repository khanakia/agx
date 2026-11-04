package render

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Tone is the colour intent of one table cell. Callers say what a value
// means (fine, close to the cap, blocked); render alone decides the ANSI
// code, so no caller ever builds an escape sequence.
type Tone int

const (
	// TonePlain renders the text as is.
	TonePlain Tone = iota
	// ToneGood is comfortably within limits (the bars' normal blue).
	ToneGood
	// ToneWarn is close to a cap (amber).
	ToneWarn
	// ToneBad is blocked: a cap reached, a login refused, a failure (red).
	ToneBad
	// ToneDim is secondary text and placeholders such as "—".
	ToneDim
)

// toneCodes maps each tone to its ANSI code; TonePlain has none.
var toneCodes = map[Tone]string{
	ToneGood: ansiBlue,
	ToneWarn: ansiYellow,
	ToneBad:  ansiRed,
	ToneDim:  ansiDim,
}

// Cell is one styled table cell.
type Cell struct {
	Text string
	Tone Tone
}

// StyledTable writes rows under a bold header with columns aligned by their
// visible width (runes), padding BEFORE colouring so ANSI codes never skew
// the alignment the way they would inside text/tabwriter. With color off it
// is byte-for-byte the plain aligned table. A row shorter than the header
// is padded with empty cells.
func StyledTable(w io.Writer, header []string, rows [][]Cell, color bool) error {
	st := styler{on: color}
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) {
				widths[i] = max(widths[i], utf8.RuneCountInString(c.Text))
			}
		}
	}
	gap := strings.Repeat(" ", tableGap)
	var b strings.Builder
	line := func(cells []Cell, headerRow bool) {
		var parts []string
		for i := range widths {
			var c Cell
			if i < len(cells) {
				c = cells[i]
			}
			text := c.Text
			if i < len(widths)-1 {
				text += strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.Text))
			}
			code := toneCodes[c.Tone]
			if headerRow {
				code = ansiBold
			}
			// Colour only the visible text, never the padding, so a coloured
			// cell occupies exactly the width of a plain one.
			if code != "" && c.Text != "" {
				text = st.wrap(code, c.Text) + text[len(c.Text):]
			}
			parts = append(parts, text)
		}
		b.WriteString(strings.TrimRight(strings.Join(parts, gap), " "))
		b.WriteByte('\n')
	}
	hdr := make([]Cell, len(header))
	for i, h := range header {
		hdr[i] = Cell{Text: h}
	}
	line(hdr, true)
	for _, r := range rows {
		line(r, false)
	}
	_, err := fmt.Fprint(w, b.String())
	return err
}
