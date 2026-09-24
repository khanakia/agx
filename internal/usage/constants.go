package usage

import "time"

// Rule: NO BARE STRINGS for any value with a closed set of choices. Every
// switch / map key in this package that refers to a limit kind, group, or
// severity picks from the constants below. The string values are the
// upstream wire format of GET /api/oauth/usage and are kept verbatim.

// Kind identifies which plan limit a Limit row measures.
type Kind string

const (
	// KindSession is the rolling 5-hour window ("Current session" on claude.ai).
	KindSession Kind = "session"
	// KindWeeklyAll is the 7-day window shared by every model ("All models").
	KindWeeklyAll Kind = "weekly_all"
	// KindWeeklyScoped is a 7-day window restricted to one model or surface
	// (e.g. "Fable"); its Scope says which.
	KindWeeklyScoped Kind = "weekly_scoped"
)

// Group is the section a Limit is rendered under.
type Group string

const (
	// GroupSession holds the 5-hour window. It renders without a heading
	// because its single row already reads "Current session".
	GroupSession Group = "session"
	// GroupWeekly holds every 7-day window.
	GroupWeekly Group = "weekly"
)

// Severity is the server's own judgement of how close a limit is to its cap.
// It drives bar colour, so the thresholds stay server-side rather than being
// re-invented here.
type Severity string

const (
	// SeverityNormal means comfortably under the cap.
	SeverityNormal Severity = "normal"
	// SeverityWarning means approaching the cap (claude.ai paints it orange).
	SeverityWarning Severity = "warning"
)

// Labels shown for the kinds whose label is not carried in the payload.
const (
	labelSession      = "Current session"
	labelWeeklyAll    = "All models"
	labelScopedFallbk = "Scoped"
	labelExtraUsage   = "Extra usage"
)

// groupHeadings maps a group to its section heading. A group missing here is
// rendered with its raw wire name so a new upstream group is never hidden.
var groupHeadings = map[Group]string{
	GroupWeekly: "Weekly limits",
}

const (
	// DefaultEndpoint is the usage endpoint Claude Code's /usage calls. It is
	// undocumented and may change shape; everything past Parse is defensive
	// for that reason.
	DefaultEndpoint = "https://api.anthropic.com/api/oauth/usage"

	// oauthBetaHeader opts a request into OAuth bearer auth on api.anthropic.com.
	// Without it the endpoint rejects the subscription access token.
	oauthBetaHeader = "anthropic-beta"
	oauthBetaValue  = "oauth-2025-04-20"

	// userAgent identifies this tool in upstream logs.
	userAgent = "claude-usage-cli"

	// maxResponseBytes caps how much of a response body is read, so a
	// misbehaving upstream cannot balloon memory. The real payload is ~3 KB.
	maxResponseBytes = 1 << 20

	// percentMax is the utilization value that means "cap reached".
	percentMax = 100.0
)

// DefaultTimeout bounds one account's fetch end to end.
const DefaultTimeout = 15 * time.Second

// DefaultBarWidth is the number of cells in a rendered progress bar.
const DefaultBarWidth = 30
