package usage

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// Report is one account's row set for rendering: who it is, and either its
// usage or the error that prevented fetching it.
type Report struct {
	// ConfigDir is the Claude Code config directory the account lives in.
	ConfigDir string
	// Email and Organization come from the account's .claude.json; either may
	// be empty when that file is missing, in which case ConfigDir identifies it.
	Email        string
	Organization string
	// Plan is a PlanLabel result such as "Max (20x)"; "" when unknown.
	Plan string
	// Usage is meaningful only when Err is nil.
	Usage Usage
	Err   error
}

// RenderOptions controls text rendering. The zero value renders plain text
// with DefaultBarWidth, relative to time.Now().
type RenderOptions struct {
	// Now is the reference for "resets in …"; zero means time.Now(). Tests
	// pin it for deterministic output.
	Now time.Time
	// Color enables ANSI colour. The caller decides (TTY, NO_COLOR, flag).
	Color bool
	// BarWidth is the number of bar cells; <= 0 means DefaultBarWidth.
	BarWidth int
	// Home, when set, is shown as "~" in config-dir paths.
	Home string
}

// ANSI styles. Colours mirror claude.ai: blue under normal, amber on
// warning, red for anything more severe or unknown.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
)

// Bar glyphs: a filled cell and an empty track cell. Distinct shapes keep the
// bar readable with colour off.
const (
	barFilled = "█"
	barTrack  = "░"
)

// Layout widths, in terminal cells.
const (
	labelWidth  = 18 // widest built-in label is "Current session" (15)
	indent      = "  "
	sectionGap  = "\n"
	errorPrefix = "error: "
)

// RenderText writes the reports as a human-readable, claude.ai-style view.
func RenderText(w io.Writer, reports []Report, opt RenderOptions) error {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if opt.BarWidth <= 0 {
		opt.BarWidth = DefaultBarWidth
	}
	st := styler{on: opt.Color}

	var b strings.Builder
	for i, r := range reports {
		if i > 0 {
			b.WriteString(sectionGap)
		}
		writeHeader(&b, st, r, opt.Home)
		if r.Err != nil {
			fmt.Fprintf(&b, "%s%s\n", indent, st.wrap(ansiRed, errorPrefix+r.Err.Error()))
			continue
		}
		writeLimits(&b, st, r.Usage, opt)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeHeader prints "email · Plan   ~/dir".
func writeHeader(b *strings.Builder, st styler, r Report, home string) {
	dir := shortenHome(r.ConfigDir, home)
	who := r.Email
	if who == "" {
		who = dir
	}
	b.WriteString(st.wrap(ansiBold, who))
	if r.Plan != "" {
		b.WriteString(st.wrap(ansiDim, "  "+r.Plan))
	}
	if who != dir {
		b.WriteString(st.wrap(ansiDim, "  "+dir))
	}
	b.WriteString("\n")
}

// writeLimits prints limits in payload order, adding a heading whenever the
// group changes (session rows have none).
func writeLimits(b *strings.Builder, st styler, u Usage, opt RenderOptions) {
	var prev Group
	for i, l := range u.Limits {
		if i == 0 || l.Group != prev {
			if h := GroupHeading(l.Group); h != "" {
				fmt.Fprintf(b, "%s%s\n", indent, st.wrap(ansiBold, h))
			}
			prev = l.Group
		}
		writeRow(b, st, l.Label(), l.Percent, l.Severity, l.ResetsAt, opt)
	}
	if u.ExtraUsage != nil && u.ExtraUsage.IsEnabled && u.ExtraUsage.Utilization != nil {
		writeRow(b, st, labelExtraUsage, *u.ExtraUsage.Utilization, SeverityNormal, nil, opt)
	}
}

// writeRow prints one "label  bar  N% used  resets in …" line.
func writeRow(b *strings.Builder, st styler, label string, pct float64, sev Severity, resetsAt *time.Time, opt RenderOptions) {
	filled, empty := barCells(pct, opt.BarWidth)
	bar := st.wrap(severityColor(sev), strings.Repeat(barFilled, filled)) + st.wrap(ansiDim, strings.Repeat(barTrack, empty))
	line := fmt.Sprintf("%s%-*s %s %4.0f%% used", indent, labelWidth, label, bar, pct)
	if resetsAt != nil {
		line += st.wrap(ansiDim, "  resets in "+FormatUntil(resetsAt.Sub(opt.Now)))
	}
	b.WriteString(line)
	b.WriteByte('\n')
}

// barCells splits width into filled and empty cells for pct (0–100, clamped).
// Any non-zero usage shows at least one filled cell so 1% is visibly not 0%.
func barCells(pct float64, width int) (filled, empty int) {
	p := math.Max(0, math.Min(percentMax, pct))
	filled = int(math.Round(p / percentMax * float64(width)))
	if filled == 0 && p > 0 {
		filled = 1
	}
	return filled, width - filled
}

// severityColor maps a severity to its bar colour. An empty severity (legacy
// payloads) counts as normal; any unrecognised non-empty value is treated as
// worse than warning, since new severities upstream are likelier to be
// "critical" than calmer than normal.
func severityColor(s Severity) string {
	switch s {
	case SeverityNormal, "":
		return ansiBlue
	case SeverityWarning:
		return ansiYellow
	default:
		return ansiRed
	}
}

// Time units for FormatUntil.
const (
	day = 24 * time.Hour
)

// FormatUntil renders a positive duration the way claude.ai does:
// "4 hr 38 min", "5 d 3 hr", "12 min". Zero or negative reads "now" — the
// window has reset and the next fetch will show fresh numbers.
func FormatUntil(d time.Duration) string {
	d = d.Truncate(time.Minute)
	switch {
	case d <= 0:
		return "now"
	case d >= day:
		days := d / day
		hours := (d % day) / time.Hour
		return fmt.Sprintf("%d d %d hr", days, hours)
	case d >= time.Hour:
		hours := d / time.Hour
		mins := (d % time.Hour) / time.Minute
		return fmt.Sprintf("%d hr %d min", hours, mins)
	default:
		return fmt.Sprintf("%d min", d/time.Minute)
	}
}

// shortenHome replaces a leading home directory with "~".
func shortenHome(path, home string) string {
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

// styler applies ANSI styles only when colour is on, so every call site stays
// branch-free.
type styler struct{ on bool }

func (s styler) wrap(code, text string) string {
	if !s.on || text == "" {
		return text
	}
	return code + text + ansiReset
}
