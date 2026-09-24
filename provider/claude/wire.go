package claude

import (
	"encoding/json"
	"time"
)

// Limit is one row of limits[] in GET /api/oauth/usage, kept in the wire
// shape so parsing stays a plain json.Unmarshal.
type Limit struct {
	Kind     string  `json:"kind"`
	Group    string  `json:"group"`
	Percent  float64 `json:"percent"`
	Severity string  `json:"severity"`
	// ResetsAt is nil when the window has no scheduled reset (observed for
	// idle windows whose clock has not started).
	ResetsAt *time.Time `json:"resets_at"`
	// Scope is nil for session / weekly_all; set for weekly_scoped.
	Scope    *Scope `json:"scope"`
	IsActive bool   `json:"is_active"`
}

// Scope narrows a weekly_scoped limit to one model or one surface.
type Scope struct {
	// Model is set when the limit covers one model family (observed:
	// display_name "Fable", id null).
	Model *ScopeRef `json:"model"`
	// Surface has only ever been observed as null, so its shape is unknown;
	// it is kept raw rather than guessed at.
	Surface json.RawMessage `json:"surface"`
}

// ScopeRef names the model a scoped limit applies to.
type ScopeRef struct {
	ID          *string `json:"id"`
	DisplayName *string `json:"display_name"`
}

// extraUsage is pay-as-you-go overage beyond the plan. Only the fields agx
// renders are typed; the whole object is passed through raw.
type extraUsage struct {
	IsEnabled bool `json:"is_enabled"`
	// Utilization is the percent of the monthly overage cap spent; nil while
	// extra usage is disabled.
	Utilization *float64 `json:"utilization"`
}

// wireResponse is the subset of GET /api/oauth/usage agx reads.
//
// The payload carries the same numbers in two shapes:
//   - limits: the current list, one row per window, with kind/scope/severity.
//   - five_hour / seven_day / seven_day_opus / seven_day_sonnet: the legacy
//     per-window objects ({utilization, resets_at}), used only when limits is
//     absent so an older or trimmed response still renders.
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
