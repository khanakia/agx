// Package usage fetches, parses and renders Claude subscription plan-limit
// utilization — the same numbers claude.ai (Settings → Usage) and Claude
// Code's /usage command show: percent of the 5-hour session window and of
// each 7-day window used, plus when each resets.
//
// Why a percentage and not "tokens left": subscription plans have no fixed
// token budget. The server only exposes utilization per window, so that is
// the most precise figure that exists.
//
// The package knows nothing about where access tokens come from (see the
// account package); it takes a token and returns typed data, so another
// program can reuse it with its own credential source.
package usage

import (
	"encoding/json"
	"time"
)

// Limit is one plan limit row, e.g. "weekly, Fable, 68% used, resets at T".
type Limit struct {
	Kind     Kind     `json:"kind"`
	Group    Group    `json:"group"`
	Percent  float64  `json:"percent"`
	Severity Severity `json:"severity"`
	// ResetsAt is nil when the window has no scheduled reset (observed for
	// idle windows whose clock has not started yet).
	ResetsAt *time.Time `json:"resets_at"`
	// Scope is nil for KindSession and KindWeeklyAll; set for KindWeeklyScoped.
	Scope *Scope `json:"scope"`
	// IsActive marks the limit that is currently binding for the account.
	IsActive bool `json:"is_active"`
}

// Scope narrows a KindWeeklyScoped limit to one model or one surface.
type Scope struct {
	// Model is set when the limit covers one model family (observed:
	// display_name "Fable", id null).
	Model *ScopeRef `json:"model"`
	// Surface has only ever been observed as null, so its shape is unknown;
	// it is kept raw and passed through to --json output untouched rather
	// than guessed at.
	Surface json.RawMessage `json:"surface"`
}

// ScopeRef names the model a scoped limit applies to.
type ScopeRef struct {
	ID          *string `json:"id"`
	DisplayName *string `json:"display_name"`
}

// ExtraUsage is pay-as-you-go overage beyond the plan. Only the fields this
// tool renders are typed; the full object is kept in Usage.ExtraUsageRaw.
type ExtraUsage struct {
	IsEnabled bool `json:"is_enabled"`
	// Utilization is the percent of the monthly overage limit spent; nil
	// while extra usage is disabled.
	Utilization *float64 `json:"utilization"`
}

// Usage is one account's parsed usage snapshot.
type Usage struct {
	Limits []Limit
	// ExtraUsage is nil when the payload omits the object.
	ExtraUsage *ExtraUsage
	// ExtraUsageRaw is the untouched extra_usage object, for --json callers
	// that want currency, credits and caps without this package modelling
	// fields it cannot verify.
	ExtraUsageRaw json.RawMessage
}
