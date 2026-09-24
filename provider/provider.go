// Package provider is agx's vendor-neutral domain model: what a provider
// (a vendor coding-agent CLI such as Claude Code or Codex) is, what a profile
// (one named way to launch it) is, and the shapes of usage, conversations and
// launch commands.
//
// Invariant: this package imports only the standard library (enforced by
// internal/archtest), so any tool can depend on it without pulling in agx's
// CLI stack. Implementations live in sub-packages (provider/claude,
// provider/codex).
package provider

import (
	"context"
	"errors"
	"time"
)

// ID names a provider. Values are also the provider's config key and the
// `provider:` field of a profile.
type ID string

// Built-in provider IDs.
const (
	Claude ID = "claude"
	Codex  ID = "codex"
)

// Billing says how a profile is paid for, which decides whether it has usage
// windows at all.
type Billing string

const (
	// BillingPlan is a subscription with usage windows (the default).
	BillingPlan Billing = "plan"
	// BillingAPI is billed per token (API key or a third-party backend such as
	// Kimi); it has no usage windows and is never chosen by auto-pick.
	BillingAPI Billing = "api"
)

// BillingValues is the canonical list, for validation and help text.
var BillingValues = []Billing{BillingPlan, BillingAPI}

// Source records where a profile came from, so `agx profiles` can always say
// whether a profile is configured or merely discovered.
type Source string

const (
	SourceConfig     Source = "config"
	SourceDiscovered Source = "discovered"
)

// Profile is one named way to launch a provider.
type Profile struct {
	Name     string
	Provider ID
	// Home is the provider's config dir (absolute, cleaned). Several profiles
	// may share a home, and therefore a login and a history store.
	Home    string
	Billing Billing
	// Args are passed to the provider binary before any per-invocation args.
	Args []string
	// Env is set in the child's environment verbatim.
	Env map[string]string
	// Secrets maps an env var name to a "scheme:ref" reference that is
	// resolved at launch (see package secret). Values never live here.
	Secrets map[string]string
	Default bool
	Source  Source
}

// Identity is what can be known about a profile's login without the network.
type Identity struct {
	Email        string
	Organization string
	// Plan is a display label such as "Max (20x)"; "" when unknown.
	Plan string
	// LoggedIn is false when no credential was found.
	LoggedIn bool
	// ExpiresAt is the access token's expiry; zero when unknown.
	ExpiresAt time.Time
	// CredentialSource says where the login was read from (file path or
	// keychain service), for `agx doctor`.
	CredentialSource string
}

// WindowKind classifies a usage window so policy code (auto-pick, colouring)
// can reason about windows across providers.
type WindowKind string

const (
	WindowSession WindowKind = "session" // short rolling window (Claude 5-hour)
	WindowWeekly  WindowKind = "weekly"  // 7-day window, all models
	WindowScoped  WindowKind = "scoped"  // window restricted to one model / surface
	WindowOther   WindowKind = "other"   // anything else (e.g. Codex 30-day)
)

// Window groups shared across providers. A group decides which section a
// window renders under; providers may emit other values, which render under
// their raw name so nothing is hidden.
const (
	// GroupSession holds short rolling windows; renders without a heading.
	GroupSession = "session"
	// GroupWeekly holds 7-day windows; renders under "Weekly limits".
	GroupWeekly = "weekly"
	// GroupPlan holds all windows of a provider that does not sub-group them
	// (e.g. Codex); renders without a heading.
	GroupPlan = "plan"
	// GroupExtra holds pay-as-you-go overage rows; renders without a heading.
	GroupExtra = "extra"
)

// Severity is the provider's own judgement of how close a window is to its
// cap. Providers that do not report one leave it empty.
type Severity string

const (
	SeverityNormal    Severity = "normal"
	SeverityWarning   Severity = "warning"
	SeverityExhausted Severity = "exhausted"
)

// Window is one usage limit: percent used plus when it resets.
type Window struct {
	Kind WindowKind `json:"kind"`
	// Group is the section the window renders under ("session", "weekly", …).
	Group string `json:"group"`
	// Label is the display name ("Current session", "All models", "Fable").
	Label    string   `json:"label"`
	Percent  float64  `json:"percent"`
	Severity Severity `json:"severity,omitempty"`
	// ResetsAt is nil when the window has no scheduled reset.
	ResetsAt *time.Time `json:"resets_at"`
	// Active marks the window currently binding the account, when known.
	Active bool `json:"active,omitempty"`
	// VendorKind is the provider's raw kind string, kept for --json callers.
	VendorKind string `json:"vendor_kind,omitempty"`
}

// Usage is one account's usage snapshot.
type Usage struct {
	Identity Identity
	Windows  []Window
	// Extra is provider-specific data passed through verbatim to --json
	// (e.g. Claude's extra_usage object); nil when absent.
	Extra []byte
}

// MaxPercent is the highest percent across all windows — the figure that
// decides whether an account can take more work, since any one window at
// 100% blocks it. Zero windows yield 0.
func (u Usage) MaxPercent() float64 {
	highest := 0.0
	for _, w := range u.Windows {
		if w.Percent > highest {
			highest = w.Percent
		}
	}
	return highest
}

// LaunchRequest describes one invocation.
type LaunchRequest struct {
	// Dir is the working directory; "" means the current one.
	Dir string
	// Args are appended after the profile's args.
	Args []string
	// ResumeID continues an existing conversation when set.
	ResumeID string
	// Env is the base environment (normally os.Environ()); the provider adds
	// and removes variables on a copy.
	Env []string
}

// Command is a fully built process invocation. It is data, not a running
// process: the CLI decides how to execute it (syscall.Exec, or printing it
// in --dry-run).
type Command struct {
	// Path is the binary name or path; resolve with exec.LookPath before exec.
	Path string
	// Args excludes argv[0].
	Args []string
	Env  []string
	Dir  string
}

// Conversation is one resumable transcript.
type Conversation struct {
	Provider ID        `json:"provider"`
	Home     string    `json:"home"`
	ID       string    `json:"id"`
	Title    string    `json:"title,omitempty"`
	Dir      string    `json:"dir"`
	Updated  time.Time `json:"updated"`
}

// ConversationQuery narrows a conversation listing.
type ConversationQuery struct {
	// Dir, when set, limits results to conversations started in this exact
	// directory.
	Dir string
	// Limit caps results (newest first); <= 0 means no cap.
	Limit int
}

// Provider is implemented once per vendor CLI.
type Provider interface {
	ID() ID
	// Binary is the executable name looked up on PATH.
	Binary() string
	// DefaultHome is the home the vendor CLI uses when no override env var
	// is set.
	DefaultHome(userHome string) string
	// Discover returns the profiles implied by what exists on disk.
	Discover(userHome string) ([]Profile, error)
	// Identity reads login state locally (no network).
	Identity(p Profile) (Identity, error)
	// Usage fetches usage windows. Returns ErrAPIBilled for BillingAPI
	// profiles and ErrNotLoggedIn / ErrExpired when the login is unusable.
	Usage(ctx context.Context, p Profile) (Usage, error)
	// Launch builds the command that starts (or resumes) the vendor CLI.
	Launch(p Profile, req LaunchRequest) (Command, error)
}

// ConversationLister is an optional upgrade for providers whose
// conversations agx can list and resume.
type ConversationLister interface {
	Conversations(ctx context.Context, home string, q ConversationQuery) ([]Conversation, error)
}

// HistoryMover is an optional upgrade for providers that key history by
// directory path, so a moved project can take its history along.
type HistoryMover interface {
	// MoveHistory re-keys history recorded for fromDir to toDir. moved is
	// false (with a nil error) when there was no history to move.
	MoveHistory(home, fromDir, toDir string) (moved bool, err error)
}

// HistoryChecker is an optional upgrade answering "is there any
// conversation started in dir?" cheaply — used by session gc and listing,
// which ask it for every folder and must not read transcripts to do so.
type HistoryChecker interface {
	HasHistory(home, dir string) bool
}

// Sentinel errors shared by all providers.
var (
	ErrAPIBilled    = errors.New("billed per token: no usage windows")
	ErrNotLoggedIn  = errors.New("not logged in")
	ErrExpired      = errors.New("login expired")
	ErrUnauthorized = errors.New("login rejected by the server")
)
