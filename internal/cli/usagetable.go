package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
)

// The token-login usage table (`agx usage --logins --table`, `--login a,b`,
// `--pick`): one compact row per login instead of a block of bars. Every
// function deciding a cell is pure (usageVerdictOf, loginTableRow) so the
// rules are table-tested without a probe.

// usageStatus is a row's outcome, a closed set shared by the table and the
// usage.list JSON (`status`).
type usageStatus string

const (
	// usageOK: the probe answered and no window is at its cap.
	usageOK usageStatus = "ok"
	// usageLimit: a window is at 100% (or the provider calls it exhausted).
	usageLimit usageStatus = "limit_reached"
	// usageRejected: the server refused the credential (HTTP 401/403).
	usageRejected usageStatus = "rejected"
	// usageExpired: the credential is past its expiry.
	usageExpired usageStatus = "expired"
	// usagePerToken: an api-key login, billed per token, with no windows.
	usagePerToken usageStatus = "per_token"
	// usageError: anything else (timeout, rate limit, bad response, a token
	// that could not be resolved).
	usageError usageStatus = "error"
)

// usageStatusLabels is how each status reads in the table.
var usageStatusLabels = map[usageStatus]string{
	usageOK:       "ok",
	usageLimit:    "limit reached",
	usageRejected: "rejected",
	usageExpired:  "expired",
	usagePerToken: "per token",
	usageError:    "error",
}

// Short reasons shown after a status ("rejected: blocked by org").
const (
	reasonOrgBlocked    = "blocked by org"
	reasonTimeout       = "timeout"
	reasonRateLimited   = "rate-limited"
	reasonInferenceOnly = "inference-only token"
	reasonNotLoggedIn   = "not logged in"
	// reasonMaxRunes caps a free-text reason so one long error cannot
	// stretch the whole table; the full message is printed under it.
	reasonMaxRunes = 40
	reasonEllipsis = "…"
)

// orgBlockedMarker is the part of the API's 403 message for an account
// whose organisation disallows OAuth ("OAuth authentication is currently
// not allowed for this organization"), matched to say why it was refused.
const orgBlockedMarker = "not allowed for this organization"

// httpStatusRe finds the "HTTP 403" a provider error carries, for a
// rejection whose message is not otherwise recognised.
var httpStatusRe = regexp.MustCompile(`HTTP (\d{3})`)

// percentFull is the utilization that means a window is at its cap.
const percentFull = 100.0

// leftWarnBelow colours LEFT amber under this many percent left (and red
// at zero), mirroring the bars' warning colour.
const leftWarnBelow = 20.0

// Table cells and headers.
const (
	cellMissingWindow = "—"
	colNum            = "#"
	colLogin          = "LOGIN"
	colAccount        = "ACCOUNT"
	colProvider       = "PROVIDER"
	colLeft           = "LEFT"
	colStatus         = "STATUS"
	// Every window gets a USED and a RESETS column, side by side, so each
	// window's own reset is visible (one shared RESETS hid the weekly reset
	// whenever the 5-hour window decided LEFT).
	col5hName     = "5H"
	colWeeklyName = "WEEKLY"
	colUsedSuffix = " USED"
	colResetSufix = " RESETS"
	// colModelSuffix follows a per-model weekly window's label ("FABLE WEEKLY").
	colModelSuffix = " WEEKLY"
	percentFmt     = "%.0f%%"
)

// usageVerdict is what one row comes to: its status, why, how much is left
// and when the window that decides it resets.
type usageVerdict struct {
	Status usageStatus
	Reason string
	// Left is 100 minus the highest used% among the windows present; nil
	// when there are no windows (never a fake 100%).
	Left *float64
	// ResetsAt is when the window deciding Left resets; nil when unknown.
	ResetsAt *time.Time
}

// usageVerdictOf computes a row's verdict from its report. Pure.
func usageVerdictOf(r render.UsageReport) usageVerdict {
	var v usageVerdict
	if w, ok := decidingWindow(r.Usage.Windows); ok {
		left := max(0, percentFull-w.Percent)
		v.Left, v.ResetsAt = &left, w.ResetsAt
	}
	v.Status, v.Reason = statusOf(r)
	return v
}

// decidingWindow is the window with the highest used%: any one window at
// its cap blocks the account, so it decides what is left. On a tie the one
// resetting LATER decides, since the account stays blocked until then.
func decidingWindow(ws []provider.Window) (provider.Window, bool) {
	var best provider.Window
	found := false
	for _, w := range ws {
		switch {
		case !found, w.Percent > best.Percent:
			best, found = w, true
		case w.Percent == best.Percent && later(w.ResetsAt, best.ResetsAt):
			best = w
		}
	}
	return best, found
}

// later reports whether a is after b; a known time is later than unknown.
func later(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	return b == nil || a.After(*b)
}

// statusOf classifies a report. Pure.
func statusOf(r render.UsageReport) (usageStatus, string) {
	err := r.Err
	switch {
	case err == nil:
		for _, w := range r.Usage.Windows {
			if w.Percent >= percentFull || w.Severity == provider.SeverityExhausted {
				return usageLimit, ""
			}
		}
		return usageOK, ""
	case errors.Is(err, provider.ErrAPIBilled):
		return usagePerToken, ""
	case errors.Is(err, provider.ErrExpired):
		return usageExpired, ""
	case errors.Is(err, provider.ErrUnauthorized):
		msg := err.Error()
		if strings.Contains(msg, orgBlockedMarker) {
			return usageRejected, reasonOrgBlocked
		}
		if m := httpStatusRe.FindStringSubmatch(msg); m != nil {
			return usageRejected, "HTTP " + m[1]
		}
		return usageRejected, ""
	case errors.Is(err, provider.ErrRateLimited):
		return usageError, reasonRateLimited
	case errors.Is(err, provider.ErrInferenceOnly):
		return usageError, reasonInferenceOnly
	case errors.Is(err, provider.ErrNotLoggedIn):
		return usageError, reasonNotLoggedIn
	case isTimeout(err):
		return usageError, reasonTimeout
	}
	return usageError, shorten(err.Error())
}

// isTimeout matches both a context deadline and an http.Client timeout.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// shorten cuts s to reasonMaxRunes runes.
func shorten(s string) string {
	r := []rune(s)
	if len(r) <= reasonMaxRunes {
		return s
	}
	return string(r[:reasonMaxRunes-1]) + reasonEllipsis
}

// loginTable is the table's header plus rows, ready for render.StyledTable.
type loginTable struct {
	Header []string
	Rows   [][]render.Cell
}

// modelColumns lists the per-model weekly windows (e.g. Fable) that any
// report actually carries, in first-seen order: a column appears only when
// a probe returned it.
func modelColumns(reports []render.UsageReport) []string {
	var labels []string
	seen := map[string]bool{}
	for _, r := range reports {
		for _, w := range r.Usage.Windows {
			if w.Kind == provider.WindowScoped && !seen[w.Label] {
				seen[w.Label] = true
				labels = append(labels, w.Label)
			}
		}
	}
	return labels
}

// buildLoginTable turns reports into the table. ordinals[i] is report i's
// position in `agx login ls` (1-based), so a row keeps its number whatever
// subset is shown. Pure: now is the reference for RESETS.
func buildLoginTable(reports []render.UsageReport, ordinals []int, now time.Time) loginTable {
	models := modelColumns(reports)
	header := []string{colNum, colLogin, colProvider, colAccount}
	for _, name := range append([]string{col5hName, colWeeklyName}, modelNames(models)...) {
		header = append(header, name+colUsedSuffix, name+colResetSufix)
	}
	header = append(header, colLeft, colStatus)
	t := loginTable{Header: header}
	for i, r := range reports {
		t.Rows = append(t.Rows, loginTableRow(r, ordinals[i], models, now))
	}
	return t
}

// modelNames is the column name of each per-model weekly window ("FABLE WEEKLY").
func modelNames(models []string) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = strings.ToUpper(m) + colModelSuffix
	}
	return out
}

// loginTableRow builds one row. A window the report lacks is "—" in both
// its USED and RESETS cells, never 0%: an account without a 5-hour window
// is not an idle one.
func loginTableRow(r render.UsageReport, ordinal int, models []string, now time.Time) []render.Cell {
	v := usageVerdictOf(r)
	account := r.Account
	if account == "" {
		account = r.Usage.Identity.Email
	}
	row := []render.Cell{
		{Text: strconv.Itoa(ordinal), Tone: render.ToneDim},
		{Text: r.Login},
		dimIfMissing(string(r.Profile.Provider)),
		dimIfMissing(account),
	}
	row = append(row, windowCells(r.Usage.Windows, now, func(w provider.Window) bool { return w.Kind == provider.WindowSession })...)
	row = append(row, windowCells(r.Usage.Windows, now, func(w provider.Window) bool { return w.Kind == provider.WindowWeekly })...)
	for _, m := range models {
		row = append(row, windowCells(r.Usage.Windows, now, func(w provider.Window) bool { return w.Kind == provider.WindowScoped && w.Label == m })...)
	}
	row = append(row, leftCell(v.Left), statusCell(v))
	return row
}

// dimIfMissing shows "—" (dim) for an empty value.
func dimIfMissing(s string) render.Cell {
	if s == "" {
		return render.Cell{Text: cellMissingWindow, Tone: render.ToneDim}
	}
	return render.Cell{Text: s}
}

// windowCells is the USED and RESETS pair of the first window matching,
// or "—" twice when the report has no such window.
func windowCells(ws []provider.Window, now time.Time, match func(provider.Window) bool) []render.Cell {
	for _, w := range ws {
		if match(w) {
			return []render.Cell{{Text: fmt.Sprintf(percentFmt, w.Percent)}, resetsCell(w.ResetsAt, now)}
		}
	}
	missing := render.Cell{Text: cellMissingWindow, Tone: render.ToneDim}
	return []render.Cell{missing, missing}
}

// leftCell colours what is left: red at zero, amber when low.
func leftCell(left *float64) render.Cell {
	if left == nil {
		return render.Cell{Text: cellMissingWindow, Tone: render.ToneDim}
	}
	tone := render.ToneGood
	switch {
	case *left <= 0:
		tone = render.ToneBad
	case *left < leftWarnBelow:
		tone = render.ToneWarn
	}
	return render.Cell{Text: fmt.Sprintf(percentFmt, *left), Tone: tone}
}

func resetsCell(at *time.Time, now time.Time) render.Cell {
	if at == nil {
		return render.Cell{Text: cellMissingWindow, Tone: render.ToneDim}
	}
	return render.Cell{Text: render.FormatUntil(at.Sub(now))}
}

// statusTones colours each status.
var statusTones = map[usageStatus]render.Tone{
	usageOK:       render.ToneGood,
	usageLimit:    render.ToneBad,
	usageRejected: render.ToneBad,
	usageExpired:  render.ToneWarn,
	usagePerToken: render.ToneDim,
	usageError:    render.ToneBad,
}

func statusCell(v usageVerdict) render.Cell {
	text := usageStatusLabels[v.Status]
	if v.Reason != "" {
		text += ": " + v.Reason
	}
	return render.Cell{Text: text, Tone: statusTones[v.Status]}
}

// writeLoginTable prints the table, then the full message of every failed
// row (the STATUS cell carries only a short reason).
func writeLoginTable(w io.Writer, reports []render.UsageReport, ordinals []int, now time.Time, color bool) error {
	t := buildLoginTable(reports, ordinals, now)
	if err := render.StyledTable(w, t.Header, t.Rows, color); err != nil {
		return err
	}
	for _, r := range reports {
		if r.Err != nil && !errors.Is(r.Err, provider.ErrAPIBilled) {
			if _, err := fmt.Fprintf(w, "  %s: %s\n", r.Login, r.Err); err != nil {
				return err
			}
		}
	}
	return nil
}

// Login list syntax for `--login`.
const (
	loginListSep = ","
	// loginAll selects every token login (`--login all`).
	loginAll = "all"
)

// parseLoginList resolves a `--login` comma list (or "all") to logins in
// `agx login ls` order, de-duplicated, with each one's 1-based position in
// that list. "home" and "auto" mean something else and are refused inside
// a list; an unknown name is a usage error. Pure.
func parseLoginList(cfg config.Config, spec string) ([]login.Login, []int, error) {
	want := map[string]bool{}
	all := false
	for _, n := range strings.Split(spec, loginListSep) {
		n = strings.TrimSpace(n)
		switch n {
		case "":
			continue
		case loginAll:
			all = true
			continue
		case login.Home, login.Auto:
			return nil, nil, usageErr(fmt.Errorf("--login %s cannot be part of a list; list token login names or %q", n, loginAll))
		}
		if _, ok := cfg.LoginByName(n); !ok {
			return nil, nil, errUnknownLogin(n)
		}
		want[n] = true
	}
	var out []login.Login
	var ord []int
	for i, l := range cfg.Logins {
		if all || want[l.Name] {
			out = append(out, l)
			ord = append(ord, i+1)
		}
	}
	if len(out) == 0 {
		return nil, nil, usageErr(errors.New("--login lists no logins (see `agx login ls`)"))
	}
	return out, ord, nil
}

// isLoginList reports whether a --login value names more than one login
// (a comma list or "all"), which selects the table.
func isLoginList(spec string) bool {
	return spec == loginAll || strings.Contains(spec, loginListSep)
}

// tableMode reports whether `agx usage` prints the login table: --table,
// --pick, or a --login list naming more than one login.
func tableMode(o *usageOpts) bool {
	return o.table || o.pick || isLoginList(o.login)
}

// errPickNoTerminal refuses --pick when nobody is at the terminal.
var errPickNoTerminal = errors.New("--pick needs an interactive terminal (stdin and stdout); name the logins instead: --login a,b or --login all")

// runLoginTable is `agx usage` in table mode: choose the logins (--logins,
// a --login list or name, or --pick), probe them all concurrently with the
// per-account timeout, and print one row each (or the usage.list JSON).
// Homes are not shown: the table is for token logins.
func (a *app) runLoginTable(ctx context.Context, cfg config.Config, o *usageOpts, names []string) error {
	ls, ord, err := a.tableLogins(cfg, o, names)
	if err != nil {
		return err
	}
	reports := a.probeLogins(ctx, ls, o.timeout)
	if o.json {
		out := make([]usageJSON, 0, len(reports))
		for _, r := range reports {
			out = append(out, toUsageJSON(r))
		}
		if err := output.JSON(a.Stdout, kindUsageList, out, len(out)); err != nil {
			return err
		}
	} else if err := writeLoginTable(a.Stdout, reports, ord, a.Now(), a.wantColor(o.color)); err != nil {
		return err
	}
	if anyFailed(reports) {
		return errPartial
	}
	return nil
}

// tableLogins resolves which logins the table shows, in `agx login ls`
// order with their positions, after --provider filtering. Exactly one of
// --logins, --login and --pick must choose them.
func (a *app) tableLogins(cfg config.Config, o *usageOpts, names []string) ([]login.Login, []int, error) {
	if len(names) > 0 {
		return nil, nil, usageErr(errors.New("the login table shows token logins; it cannot be combined with profile names"))
	}
	sources := 0
	for _, set := range []bool{o.logins, o.pick, o.login != "" && o.login != login.Home} {
		if set {
			sources++
		}
	}
	switch {
	case sources == 0:
		return nil, nil, usageErr(errors.New("--table shows token logins: add --logins, --login a,b (or all) or --pick"))
	case sources > 1:
		return nil, nil, usageErr(errors.New("choose the logins one way: --logins, --login or --pick"))
	}
	var (
		ls  []login.Login
		ord []int
		err error
	)
	switch {
	case o.logins:
		for i, l := range cfg.Logins {
			ls, ord = append(ls, l), append(ord, i+1)
		}
	case o.pick:
		ls, ord, err = a.pickLogins(cfg, o.provider)
	default:
		ls, ord, err = parseLoginList(cfg, o.login)
	}
	if err != nil {
		return nil, nil, err
	}
	var fl []login.Login
	var fo []int
	for i, l := range ls {
		if o.provider == "" || string(l.Provider) == o.provider {
			fl, fo = append(fl, l), append(fo, ord[i])
		}
	}
	if len(fl) == 0 {
		return nil, nil, usageErr(errors.New("no token logins to show (add one with `agx login add NAME`)"))
	}
	return fl, fo, nil
}

// pickLogins shows the --pick checklist of token logins (only those of
// providerID, when set) and returns the chosen ones in `agx login ls`
// order. Esc / q is exit 1, quietly, like every other picker.
func (a *app) pickLogins(cfg config.Config, providerID string) ([]login.Login, []int, error) {
	if a.Interactive == nil || !a.Interactive() {
		return nil, nil, usageErr(errPickNoTerminal)
	}
	var cands []login.Login
	var ord []int
	var items []string
	for i, l := range cfg.Logins {
		if providerID != "" && string(l.Provider) != providerID {
			continue
		}
		cands, ord = append(cands, l), append(ord, i+1)
	}
	// Names padded to one width so the account labels line up.
	width := 0
	for _, l := range cands {
		width = max(width, len(l.Name))
	}
	for _, l := range cands {
		items = append(items, strings.TrimRight(fmt.Sprintf("%-*s  %s", width, l.Name, l.Account), " "))
	}
	if len(cands) == 0 {
		return nil, nil, usageErr(errors.New("no token logins to pick from (add one with `agx login add NAME`)"))
	}
	var idx []int
	var err error
	switch p := a.Picker.(type) {
	case CheckPicker:
		idx, err = p.PickChecked(pickPrompt, items)
	case MultiPicker:
		idx, err = p.PickMany(pickPrompt, items)
	default:
		var i int
		i, err = a.Picker.Pick(pickPrompt, items)
		idx = []int{i}
	}
	if errors.Is(err, ErrCancelled) {
		return nil, nil, &ExitError{Code: ExitFailure, Err: err}
	}
	if err != nil {
		return nil, nil, err
	}
	// Indexes come back in the order marked; the table keeps login ls order.
	chosen := make([]bool, len(cands))
	for _, n := range idx {
		if n >= 0 && n < len(cands) {
			chosen[n] = true
		}
	}
	var ls []login.Login
	var lo []int
	for i, c := range chosen {
		if c {
			ls, lo = append(ls, cands[i]), append(lo, ord[i])
		}
	}
	return ls, lo, nil
}
