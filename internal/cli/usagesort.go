package cli

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/provider"
)

// `agx usage --sort` and `--tz`: the order of the login table's rows and
// the zone its reset days are read in. Parsing and sorting are pure so the
// rules are table-tested without a probe.

// Sort keys of --sort.
const (
	sortLogin       = "login"
	sortLeft        = "left"
	sort5h          = "5h"
	sort5hReset     = "5h-reset"
	sortWeekly      = "weekly"
	sortWeeklyReset = "weekly-reset"
	sortDay         = "day"
	sortStatus      = "status"
)

// sortKeyNames lists the valid keys, in the order the error message shows.
var sortKeyNames = []string{sortLogin, sortLeft, sort5h, sort5hReset, sortWeekly, sortWeeklyReset, sortDay, sortStatus}

// --sort syntax.
const (
	sortKeySep  = ","
	sortDescPre = "-"
)

// sortKey is one --sort key: its name and whether it runs high to low.
type sortKey struct {
	Name string
	Desc bool
}

// parseSortKeys reads a --sort value ("day,-left"). An empty value is no
// sort; an unknown or empty key is a usage error listing the valid keys.
// Pure.
func parseSortKeys(spec string) ([]sortKey, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}
	var keys []sortKey
	for _, part := range strings.Split(spec, sortKeySep) {
		part = strings.TrimSpace(part)
		k := sortKey{Name: strings.TrimPrefix(part, sortDescPre)}
		k.Desc = k.Name != part
		if !slices.Contains(sortKeyNames, k.Name) {
			return nil, usageErr(fmt.Errorf("--sort: unknown key %q; valid keys: %s (prefix %s for high to low)",
				part, strings.Join(sortKeyNames, ", "), sortDescPre))
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// parseTZ resolves --tz: an IANA zone name, or the machine's local zone
// when empty. An unknown zone is a usage error.
func parseTZ(name string) (*time.Location, error) {
	if name == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, usageErr(fmt.Errorf("--tz: unknown time zone %q (use an IANA name such as Asia/Kolkata)", name))
	}
	return loc, nil
}

// errSortNeedsTable refuses --sort outside the login table.
var errSortNeedsTable = errors.New("--sort orders the login table: add --table with --logins, --login a,b (or all) or --pick")

// statusRank orders STATUS for the status key: ok first, then the limits,
// then the failures.
var statusRank = map[usageStatus]int{
	usageOK:       0,
	usageLimit:    1,
	usageExpired:  2,
	usageRejected: 3,
	usageError:    4,
	usagePerToken: 5,
}

// sortValue is one row's value for a key: a number (percent, unix time,
// weekday, rank) or a string; ok is false when the row has none.
type sortValue struct {
	num float64
	str string
	ok  bool
}

// windowOf is the first window of kind, if any.
func windowOf(ws []provider.Window, kind provider.WindowKind) (provider.Window, bool) {
	for _, w := range ws {
		if w.Kind == kind {
			return w, true
		}
	}
	return provider.Window{}, false
}

// mondayFirst numbers weekdays Mon=0 .. Sun=6, the calendar order of `day`.
func mondayFirst(d time.Weekday) int { return (int(d) + 6) % 7 }

// sortValueOf is r's value for key, in loc for `day`.
func sortValueOf(r render.UsageReport, key string, loc *time.Location) sortValue {
	num := func(f float64) sortValue { return sortValue{num: f, ok: true} }
	at := func(t *time.Time) sortValue {
		if t == nil {
			return sortValue{}
		}
		return num(float64(t.Unix()))
	}
	session, hasSession := windowOf(r.Usage.Windows, provider.WindowSession)
	weekly, hasWeekly := windowOf(r.Usage.Windows, provider.WindowWeekly)
	switch key {
	case sortLogin:
		return sortValue{str: r.Login, ok: true}
	case sortLeft:
		if v := usageVerdictOf(r); v.Left != nil {
			return num(*v.Left)
		}
	case sort5h:
		if hasSession {
			return num(session.Percent)
		}
	case sort5hReset:
		if hasSession {
			return at(session.ResetsAt)
		}
	case sortWeekly:
		if hasWeekly {
			return num(weekly.Percent)
		}
	case sortWeeklyReset:
		if hasWeekly {
			return at(weekly.ResetsAt)
		}
	case sortDay:
		if hasWeekly && weekly.ResetsAt != nil {
			return num(float64(mondayFirst(weekly.ResetsAt.In(loc).Weekday())))
		}
	case sortStatus:
		return num(float64(statusRank[usageVerdictOf(r).Status]))
	}
	return sortValue{}
}

// compareSortValues orders two values for one key; a missing value goes
// last whichever the direction.
func compareSortValues(a, b sortValue, desc bool) int {
	switch {
	case !a.ok && !b.ok:
		return 0
	case !a.ok:
		return 1
	case !b.ok:
		return -1
	}
	c := cmp.Or(cmp.Compare(a.num, b.num), strings.Compare(a.str, b.str))
	if desc {
		return -c
	}
	return c
}

// sortLoginRows orders reports (and their ordinals with them, so # keeps
// each login's own number) by keys, stably. No keys leaves the order
// unchanged. Pure: the inputs are not modified.
func sortLoginRows(reports []render.UsageReport, ordinals []int, keys []sortKey, loc *time.Location) ([]render.UsageReport, []int) {
	if len(keys) == 0 {
		return reports, ordinals
	}
	idx := make([]int, len(reports))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(i, j int) int {
		for _, k := range keys {
			c := compareSortValues(sortValueOf(reports[i], k.Name, loc), sortValueOf(reports[j], k.Name, loc), k.Desc)
			if c != 0 {
				return c
			}
		}
		return 0
	})
	rs := make([]render.UsageReport, len(idx))
	ords := make([]int, len(idx))
	for n, i := range idx {
		rs[n], ords[n] = reports[i], ordinals[i]
	}
	return rs, ords
}
