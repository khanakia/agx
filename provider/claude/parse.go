package claude

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/khanakia/agx/provider"
)

// ErrNoLimits means the payload parsed but carried no limit data in either
// the current or the legacy shape. It is an error rather than an empty result
// so an upstream shape change is never mistaken for "0% used".
var ErrNoLimits = errors.New("claude: usage response has no limits")

// ParseUsage decodes a usage response body into provider windows.
//
// Invariant: a nil error always comes with at least one window.
func ParseUsage(body []byte) (provider.Usage, error) {
	var w wireResponse
	if err := json.Unmarshal(body, &w); err != nil {
		return provider.Usage{}, fmt.Errorf("claude: decode usage: %w", err)
	}
	limits := w.Limits
	if len(limits) == 0 {
		limits = legacyLimits(w)
	}
	if len(limits) == 0 {
		return provider.Usage{}, ErrNoLimits
	}

	u := provider.Usage{Windows: make([]provider.Window, 0, len(limits)+1)}
	for _, l := range limits {
		u.Windows = append(u.Windows, toWindow(l))
	}

	if len(w.ExtraUsage) > 0 && string(w.ExtraUsage) != "null" {
		var extra extraUsage
		if err := json.Unmarshal(w.ExtraUsage, &extra); err != nil {
			return provider.Usage{}, fmt.Errorf("claude: decode extra_usage: %w", err)
		}
		u.Extra = w.ExtraUsage
		if extra.IsEnabled && extra.Utilization != nil {
			u.Windows = append(u.Windows, provider.Window{
				Kind: provider.WindowOther, Group: provider.GroupExtra, Label: labelExtraUsage,
				Percent: *extra.Utilization, VendorKind: vendorKindExtra,
			})
		}
	}
	return u, nil
}

// Extra-usage pseudo-window naming (not a wire value: agx synthesises this
// row from extra_usage.utilization).
const (
	labelExtraUsage = "Extra usage"
	vendorKindExtra = "extra_usage"
)

// toWindow maps one wire limit to the neutral window model.
func toWindow(l Limit) provider.Window {
	return provider.Window{
		Kind:       windowKind(l.Kind),
		Group:      l.Group,
		Label:      l.Label(),
		Percent:    l.Percent,
		Severity:   severity(l.Severity),
		ResetsAt:   l.ResetsAt,
		Active:     l.IsActive,
		VendorKind: l.Kind,
	}
}

// windowKind classifies a wire kind. Unknown kinds become WindowOther rather
// than being dropped, so a window added upstream still shows up.
func windowKind(k string) provider.WindowKind {
	switch k {
	case kindSession:
		return provider.WindowSession
	case kindWeeklyAll:
		return provider.WindowWeekly
	case kindWeeklyScoped:
		return provider.WindowScoped
	default:
		return provider.WindowOther
	}
}

// severity maps a wire severity. Empty stays empty (legacy payloads); an
// unrecognised non-empty value is treated as exhausted, since a new upstream
// severity is likelier to be "critical" than calmer than normal.
func severity(s string) provider.Severity {
	switch s {
	case "":
		return ""
	case severityNormal:
		return provider.SeverityNormal
	case severityWarning:
		return provider.SeverityWarning
	default:
		return provider.SeverityExhausted
	}
}

// legacyLimits rebuilds limit rows from the per-window objects. Severity is
// left empty because the legacy shape does not carry it.
func legacyLimits(w wireResponse) []Limit {
	var out []Limit
	add := func(win *wireWindow, kind, group, model string) {
		if win == nil {
			return
		}
		l := Limit{Kind: kind, Group: group, Percent: win.Utilization, ResetsAt: win.ResetsAt}
		if model != "" {
			name := model
			l.Scope = &Scope{Model: &ScopeRef{DisplayName: &name}}
		}
		out = append(out, l)
	}
	add(w.FiveHour, kindSession, groupSession, "")
	add(w.SevenDay, kindWeeklyAll, groupWeekly, "")
	add(w.SevenDayOpus, kindWeeklyScoped, groupWeekly, labelOpus)
	add(w.SevenDaySonnet, kindWeeklyScoped, groupWeekly, labelSonnet)
	return out
}
