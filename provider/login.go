package provider

import (
	"context"
	"time"
)

// LoginKind names the type of credential a login carries. It decides which
// env var the vendor CLI reads it from and how the account is billed, so it
// is a closed set every provider maps onto its own variables.
type LoginKind string

const (
	// LoginOAuthToken is a long-lived plan token (Claude `setup-token`): it
	// spends the account's plan quota and is never refreshed.
	LoginOAuthToken LoginKind = "oauth-token"
	// LoginAPIKey is a pay-per-token API key.
	LoginAPIKey LoginKind = "api-key"
)

// LoginKindValues is the canonical list, for validation and help text.
var LoginKindValues = []LoginKind{LoginOAuthToken, LoginAPIKey}

// LoginLauncher is an optional upgrade for providers that can run on a
// credential other than the one stored in the home (logins spec §10). A
// provider without it is never forced to pretend: agx refuses logins for it.
type LoginLauncher interface {
	// LoginKinds lists the kinds this provider can launch with; the first
	// is the default kind for `agx login add`.
	LoginKinds() []LoginKind
	// LoginEnv returns the env vars to set and remove so the vendor CLI
	// uses value instead of the home login. Called with an empty value it
	// still names every variable, which is how callers learn which
	// variables a login owns (for conflict checks and --dry-run).
	LoginEnv(kind LoginKind, value string) (set map[string]string, unset []string)
}

// LoginLifetimer is an optional upgrade reporting how long a credential of
// kind stays valid after it was issued, so `agx login ls` and `agx doctor`
// can warn before it stops working. Zero means it does not expire.
type LoginLifetimer interface {
	LoginLifetime(kind LoginKind) time.Duration
}

// LoginUsageReader is a further optional upgrade: usage and identity for a
// raw credential. Absent → the login shows "usage not readable".
type LoginUsageReader interface {
	LoginUsage(ctx context.Context, kind LoginKind, value string) (Usage, Identity, error)
}

// ApplyLoginEnv returns a copy of env with a login's variables applied:
// every unset name removed, then every set name set. The input slice is
// never modified.
func ApplyLoginEnv(env []string, set map[string]string, unset []string) []string {
	out := append([]string{}, env...)
	for _, k := range unset {
		out = UnsetEnv(out, k)
	}
	for k, v := range set {
		out = SetEnv(out, k, v)
	}
	return out
}
