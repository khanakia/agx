package claude

import (
	"regexp"
	"strings"
)

// Label is the human name of a limit row, matching claude.ai's wording.
//
// Unknown kinds fall back to their raw wire value rather than being dropped,
// so a window added upstream still shows up (just with a less pretty name).
func (l Limit) Label() string {
	switch l.Kind {
	case kindSession:
		return labelSession
	case kindWeeklyAll:
		return labelWeeklyAll
	case kindWeeklyScoped:
		if l.Scope != nil && l.Scope.Model != nil && l.Scope.Model.DisplayName != nil && *l.Scope.Model.DisplayName != "" {
			return *l.Scope.Model.DisplayName
		}
		return labelScopedFallbk
	default:
		return l.Kind
	}
}

// tierMultiplier pulls the "20x" out of a rate-limit tier such as
// "default_claude_max_20x".
var tierMultiplier = regexp.MustCompile(`_(\d+x)$`)

// PlanLabel renders the plan name the way claude.ai does, e.g. "Max (20x)".
//
// Inputs are the subscriptionType and rateLimitTier fields Claude Code stores
// beside the OAuth token. An empty subscription yields "" so the caller can
// omit the plan rather than print a blank one.
func PlanLabel(subscriptionType, rateLimitTier string) string {
	if subscriptionType == "" {
		return ""
	}
	name := strings.ToUpper(subscriptionType[:1]) + subscriptionType[1:]
	if m := tierMultiplier.FindStringSubmatch(rateLimitTier); m != nil {
		return name + " (" + m[1] + ")"
	}
	return name
}
