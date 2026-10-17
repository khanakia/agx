package cli

import (
	"errors"
	"fmt"
)

// ellipsis marks truncated text.
const ellipsis = "…"

// truncate shortens s to n runes, marking the cut.
//
// Invariant: n >= 1 (every caller passes a column-width constant); the
// ellipsis takes the last of the n runes.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + ellipsis
}

// errUnknownProfile is the usage error (exit 2) for a profile name that no
// profile has. Every command that takes a profile name uses it, so the
// wording and the `agx profiles` hint cannot drift between commands.
func errUnknownProfile(name string) error {
	return usageErr(fmt.Errorf("unknown profile %q (see `agx profiles`)", name))
}

// errUnknownLogin is the usage error (exit 2) for a login name that no
// configured login has; shared for the same reason as errUnknownProfile.
func errUnknownLogin(name string) error {
	return usageErr(fmt.Errorf("unknown login %q (see `agx login ls`)", name))
}

// pickOne runs the picker and turns Esc into exit 1, so a dismissed picker
// ends the command quietly instead of reading as a failure to report.
// Callers that treat Esc as "go back" (the chat level of `agx ws`) call the
// Picker directly instead.
func (a *app) pickOne(prompt string, items []string) (int, error) {
	i, err := a.Picker.Pick(prompt, items)
	if errors.Is(err, ErrCancelled) {
		return 0, &ExitError{Code: ExitFailure, Err: err}
	}
	return i, err
}
