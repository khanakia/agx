// Package pick implements agx's auto-pick policy: given candidate profiles
// and their usage, choose the one with the most headroom.
//
// Policy (spec §7): score = the highest percent across all of a profile's
// windows, because any single window at its cap blocks work; the lowest score
// wins; ties go to candidate order (config order). Profiles whose usage could
// not be fetched are ineligible and reported with their reason.
//
// Pure: no I/O, so the policy is trivially testable and reusable.
package pick

import (
	"errors"
	"fmt"
	"strings"

	"github.com/khanakia/agx/provider"
)

// Candidate is one profile with its usage fetch result.
type Candidate struct {
	Profile provider.Profile
	Usage   provider.Usage
	// Err is the usage fetch error; a non-nil Err makes the candidate
	// ineligible.
	Err error
}

// Scored is an eligible candidate with its score.
type Scored struct {
	Profile provider.Profile
	Score   float64
}

// Result is the decision plus everything needed to explain it.
type Result struct {
	Chosen provider.Profile
	Score  float64
	// Others are the remaining eligible candidates, best first.
	Others []Scored
	// Skipped maps profile name to why it was ineligible.
	Skipped map[string]error
}

// ErrNoCandidate means no profile was eligible; the wrapped message lists
// every candidate's reason.
var ErrNoCandidate = errors.New("pick: no eligible profile")

// Best applies the policy to cands (in config order).
func Best(cands []Candidate) (Result, error) {
	res := Result{Skipped: map[string]error{}}
	var eligible []Scored
	for _, c := range cands {
		switch {
		case c.Err != nil:
			res.Skipped[c.Profile.Name] = c.Err
		case c.Profile.Billing == provider.BillingAPI:
			res.Skipped[c.Profile.Name] = provider.ErrAPIBilled
		default:
			eligible = append(eligible, Scored{Profile: c.Profile, Score: c.Usage.MaxPercent()})
		}
	}
	if len(eligible) == 0 {
		return res, fmt.Errorf("%w: %s", ErrNoCandidate, reasons(cands, res.Skipped))
	}
	// Stable selection: strictly-lower score wins, so equal scores keep
	// config order.
	best := 0
	for i := 1; i < len(eligible); i++ {
		if eligible[i].Score < eligible[best].Score {
			best = i
		}
	}
	res.Chosen, res.Score = eligible[best].Profile, eligible[best].Score
	for i, e := range eligible {
		if i != best {
			res.Others = append(res.Others, e)
		}
	}
	sortScored(res.Others)
	return res, nil
}

// Explain renders the decision in one line for stderr, e.g.
// "auto → work (max 5%) over personal (max 77%)".
func (r Result) Explain() string {
	var b strings.Builder
	fmt.Fprintf(&b, "auto → %s (max %.0f%%)", r.Chosen.Name, r.Score)
	if len(r.Others) > 0 {
		parts := make([]string, 0, len(r.Others))
		for _, o := range r.Others {
			parts = append(parts, fmt.Sprintf("%s (max %.0f%%)", o.Profile.Name, o.Score))
		}
		b.WriteString(" over " + strings.Join(parts, ", "))
	}
	return b.String()
}

// sortScored orders by score ascending, keeping input order on ties
// (insertion sort: lists are a handful of profiles).
func sortScored(s []Scored) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Score < s[j-1].Score; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// reasons lists skipped candidates in input order.
func reasons(cands []Candidate, skipped map[string]error) string {
	parts := make([]string, 0, len(skipped))
	for _, c := range cands {
		if err, ok := skipped[c.Profile.Name]; ok {
			parts = append(parts, c.Profile.Name+": "+err.Error())
		}
	}
	if len(parts) == 0 {
		return "no profiles"
	}
	return strings.Join(parts, "; ")
}
