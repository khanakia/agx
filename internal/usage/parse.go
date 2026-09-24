package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrNoLimits means the payload parsed but carried no limit data in either
// the current or the legacy shape. It is an error rather than an empty
// result so an upstream shape change is never mistaken for "0% used".
var ErrNoLimits = errors.New("usage: response has no limits")

// wireResponse is the subset of GET /api/oauth/usage this tool reads.
//
// The payload carries the same numbers in two shapes:
//   - limits: the current list, one row per window, with kind/scope/severity.
//   - five_hour / seven_day / seven_day_opus / seven_day_sonnet: the legacy
//     per-window objects ({utilization, resets_at}). Used only when limits
//     is absent, so an older or trimmed response still renders.
//
// Every other top-level key (dozens of codenamed, usually-null windows) is
// deliberately ignored: they have no stable meaning to label.
type wireResponse struct {
	Limits         []Limit         `json:"limits"`
	ExtraUsage     json.RawMessage `json:"extra_usage"`
	FiveHour       *wireWindow     `json:"five_hour"`
	SevenDay       *wireWindow     `json:"seven_day"`
	SevenDayOpus   *wireWindow     `json:"seven_day_opus"`
	SevenDaySonnet *wireWindow     `json:"seven_day_sonnet"`
}

// wireWindow is one legacy per-window object.
type wireWindow struct {
	Utilization float64    `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at"`
}

// Legacy scoped windows carry no display name, so name them here.
const (
	legacyOpusName   = "Opus"
	legacySonnetName = "Sonnet"
)

// Parse decodes a usage response body.
//
// Invariant: a nil error always comes with at least one Limit — see
// ErrNoLimits.
func Parse(body []byte) (Usage, error) {
	var w wireResponse
	if err := json.Unmarshal(body, &w); err != nil {
		return Usage{}, fmt.Errorf("usage: decode response: %w", err)
	}

	u := Usage{Limits: w.Limits}
	if len(u.Limits) == 0 {
		u.Limits = legacyLimits(w)
	}
	if len(u.Limits) == 0 {
		return Usage{}, ErrNoLimits
	}

	if len(w.ExtraUsage) > 0 && string(w.ExtraUsage) != "null" {
		var extra ExtraUsage
		if err := json.Unmarshal(w.ExtraUsage, &extra); err != nil {
			return Usage{}, fmt.Errorf("usage: decode extra_usage: %w", err)
		}
		u.ExtraUsage = &extra
		u.ExtraUsageRaw = w.ExtraUsage
	}
	return u, nil
}

// legacyLimits rebuilds Limit rows from the per-window objects. Severity is
// left empty because the legacy shape does not carry it; the renderer treats
// an empty severity as normal.
func legacyLimits(w wireResponse) []Limit {
	var out []Limit
	add := func(win *wireWindow, kind Kind, group Group, model string) {
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
	add(w.FiveHour, KindSession, GroupSession, "")
	add(w.SevenDay, KindWeeklyAll, GroupWeekly, "")
	add(w.SevenDayOpus, KindWeeklyScoped, GroupWeekly, legacyOpusName)
	add(w.SevenDaySonnet, KindWeeklyScoped, GroupWeekly, legacySonnetName)
	return out
}
