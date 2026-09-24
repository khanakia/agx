// Package render draws agx's human-readable terminal output: usage bars in
// the claude.ai style and simple aligned tables. It writes only to an
// io.Writer and knows nothing about where the data came from.
package render

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/khanakia/agx/provider"
)

// UsageReport is one login's row set: who it is and either its windows or the
// error that prevented fetching them.
type UsageReport struct {
	Profile provider.Profile
	Usage   provider.Usage
	Err     error
}

// Options controls rendering. The zero value renders plain text with
// DefaultBarWidth relative to time.Now().
type Options struct {
	// Now is the reference for "resets in …"; zero means time.Now().
	Now time.Time
	// Color enables ANSI colour; the caller decides (TTY, NO_COLOR, flag).
	Color bool
	// BarWidth is the number of bar cells; <= 0 means DefaultBarWidth.
	BarWidth int
	// Home, when set, is shown as "~" in paths.
	Home string
}

// DefaultBarWidth is the number of cells in a usage bar.
const DefaultBarWidth = 30

// percentMax is the utilization that means "cap reached".
const percentMax = 100.0

// ANSI styles. Colours mirror claude.ai: blue normal, amber warning, red for
// anything more severe.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
)

// Bar glyphs: distinct shapes keep the bar readable with colour off.
const (
	barFilled = "█"
	barTrack  = "░"
)

// Layout.
const (
	labelWidth  = 18
	indent      = "  "
	errorPrefix = "error: "
	apiBilled   = "billed per token — no usage windows"
)

// groupHeadings maps a window group to its section heading; the session group
// and single-group providers render without one. An unknown group falls back
// to its raw name so a new upstream group is never hidden.
var groupHeadings = map[string]string{
	provider.GroupWeekly: "Weekly limits",
}

// noHeadingGroups render their rows without a heading.
var noHeadingGroups = map[string]bool{provider.GroupSession: true, provider.GroupPlan: true, provider.GroupExtra: true}

// Usage writes the reports as the claude.ai-style bars view.
func Usage(w io.Writer, reports []UsageReport, opt Options) error {
	opt = opt.withDefaults()
	st := styler{on: opt.Color}
	var b strings.Builder
	for i, r := range reports {
		if i > 0 {
			b.WriteByte('\n')
		}
		writeHeader(&b, st, r, opt.Home)
		switch {
		case r.Err != nil && isAPIBilled(r.Err):
			fmt.Fprintf(&b, "%s%s\n", indent, st.wrap(ansiDim, apiBilled))
		case r.Err != nil:
			fmt.Fprintf(&b, "%s%s\n", indent, st.wrap(ansiRed, errorPrefix+r.Err.Error()))
		default:
			writeWindows(&b, st, r.Usage.Windows, opt)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeHeader prints "email  Plan  profile · provider  ~/home".
func writeHeader(b *strings.Builder, st styler, r UsageReport, home string) {
	who := r.Usage.Identity.Email
	if who == "" {
		who = r.Profile.Name
	}
	b.WriteString(st.wrap(ansiBold, who))
	if plan := r.Usage.Identity.Plan; plan != "" {
		b.WriteString(st.wrap(ansiDim, "  "+plan))
	}
	b.WriteString(st.wrap(ansiDim, fmt.Sprintf("  %s · %s  %s", r.Profile.Name, r.Profile.Provider, ShortenHome(r.Profile.Home, home))))
	b.WriteByte('\n')
}

// writeWindows prints windows in order, adding a heading when the group
// changes.
func writeWindows(b *strings.Builder, st styler, ws []provider.Window, opt Options) {
	prev := ""
	for i, win := range ws {
		if i == 0 || win.Group != prev {
			if h := heading(win.Group); h != "" {
				fmt.Fprintf(b, "%s%s\n", indent, st.wrap(ansiBold, h))
			}
			prev = win.Group
		}
		writeRow(b, st, win, opt)
	}
}

// heading returns the section title for a group, or "".
func heading(group string) string {
	if noHeadingGroups[group] {
		return ""
	}
	if h, ok := groupHeadings[group]; ok {
		return h
	}
	return group
}

// writeRow prints one "label  bar  N% used  resets in …" line.
func writeRow(b *strings.Builder, st styler, win provider.Window, opt Options) {
	filled, empty := BarCells(win.Percent, opt.BarWidth)
	bar := st.wrap(severityColor(win.Severity), strings.Repeat(barFilled, filled)) + st.wrap(ansiDim, strings.Repeat(barTrack, empty))
	line := fmt.Sprintf("%s%-*s %s %4.0f%% used", indent, labelWidth, win.Label, bar, win.Percent)
	if win.ResetsAt != nil {
		line += st.wrap(ansiDim, "  resets in "+FormatUntil(win.ResetsAt.Sub(opt.Now)))
	}
	b.WriteString(line)
	b.WriteByte('\n')
}

// BarCells splits width into filled and empty cells for pct (clamped to
// 0–100). Any non-zero usage shows at least one filled cell so 1% is visibly
// not 0%.
func BarCells(pct float64, width int) (filled, empty int) {
	p := math.Max(0, math.Min(percentMax, pct))
	filled = int(math.Round(p / percentMax * float64(width)))
	if filled == 0 && p > 0 {
		filled = 1
	}
	return filled, width - filled
}

// severityColor maps a severity to its bar colour; empty (unreported) counts
// as normal.
func severityColor(s provider.Severity) string {
	switch s {
	case provider.SeverityNormal, "":
		return ansiBlue
	case provider.SeverityWarning:
		return ansiYellow
	default:
		return ansiRed
	}
}

const day = 24 * time.Hour

// FormatUntil renders a duration the way claude.ai does: "4 hr 38 min",
// "5 d 3 hr", "12 min"; zero or negative reads "now".
func FormatUntil(d time.Duration) string {
	d = d.Truncate(time.Minute)
	switch {
	case d <= 0:
		return "now"
	case d >= day:
		return fmt.Sprintf("%d d %d hr", d/day, (d%day)/time.Hour)
	case d >= time.Hour:
		return fmt.Sprintf("%d hr %d min", d/time.Hour, (d%time.Hour)/time.Minute)
	default:
		return fmt.Sprintf("%d min", d/time.Minute)
	}
}

// FormatAgo renders how long ago t was, coarsely: "3 min ago", "2 hr ago",
// "5 d ago".
func FormatAgo(now, t time.Time) string {
	d := now.Sub(t).Truncate(time.Minute)
	switch {
	case d < time.Minute:
		return "just now"
	case d >= day:
		return fmt.Sprintf("%d d ago", d/day)
	case d >= time.Hour:
		return fmt.Sprintf("%d hr ago", d/time.Hour)
	default:
		return fmt.Sprintf("%d min ago", d/time.Minute)
	}
}

// ShortenHome replaces a leading home directory with "~".
func ShortenHome(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+"/"); ok {
		return "~/" + rest
	}
	return path
}

// tableGap is the space between table columns.
const tableGap = 2

// Table writes rows under a header, columns aligned with two spaces.
func Table(w io.Writer, header []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, tableGap, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(header, "\t")); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(r, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func (o Options) withDefaults() Options {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.BarWidth <= 0 {
		o.BarWidth = DefaultBarWidth
	}
	return o
}

// isAPIBilled reports the expected "no windows" state, which renders dim
// rather than as an error.
func isAPIBilled(err error) bool {
	return errors.Is(err, provider.ErrAPIBilled)
}

// styler applies ANSI styles only when colour is on.
type styler struct{ on bool }

func (s styler) wrap(code, text string) string {
	if !s.on || text == "" {
		return text
	}
	return code + text + ansiReset
}
