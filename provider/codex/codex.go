// Package codex implements provider.Provider for OpenAI's Codex CLI: reading
// the ChatGPT login Codex stored in <home>/auth.json, fetching plan usage,
// building launch / resume commands, and listing conversations.
//
// Like the Claude provider it only READS the login and never refreshes it
// (spec ADR AGX-002). Codex keys conversations by id, not by directory, so
// it does not implement provider.HistoryMover.
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/khanakia/agx/provider"
)

// Rule: NO BARE STRINGS for Codex's on-disk names, env vars and wire values;
// they are Codex conventions (verified against codex-cli 0.153) kept verbatim.
const (
	// Binary is the Codex CLI executable name.
	Binary = "codex"
	// HomeEnv selects a non-default Codex home.
	HomeEnv = "CODEX_HOME"
	// defaultDirName is the home used when CODEX_HOME is unset.
	defaultDirName = ".codex"
	// authFile holds the login.
	authFile = "auth.json"
	// sessionsDir holds rollouts as sessions/YYYY/MM/DD/rollout-*.jsonl.
	sessionsDir       = "sessions"
	rolloutPrefix     = "rollout-"
	transcriptExt     = ".jsonl"
	recordSessionMeta = "session_meta"
	// resumeSubcommand continues a conversation by id.
	resumeSubcommand = "resume"
	// defaultProfileName names the discovered profile for ~/.codex.
	defaultProfileName = "codex"

	// DefaultEndpoint is the ChatGPT usage endpoint the Codex CLI's /status
	// reads. Undocumented; parsing is defensive.
	DefaultEndpoint  = "https://chatgpt.com/backend-api/wham/usage"
	accountIDHeader  = "ChatGPT-Account-Id"
	userAgent        = "agx-cli"
	maxResponseBytes = 1 << 20
	snippetMax       = 200
	headScanBytes    = 256 << 10
	// DefaultTimeout bounds one usage fetch end to end.
	DefaultTimeout = 15 * time.Second
)

// percentFull is the utilization that means "cap reached".
const percentFull = 100.0

// Window lengths Codex reports in limit_window_seconds, mapped to labels.
const (
	fiveHourSeconds  = 5 * 60 * 60
	weekSeconds      = 7 * 24 * 60 * 60
	thirtyDaySeconds = 30 * 24 * 60 * 60
	daySeconds       = 24 * 60 * 60
)

// Window labels and groups agx assigns (Codex does not name its windows).
const (
	labelFiveHour   = "5-hour"
	labelWeekly     = "Weekly"
	labelThirtyDay  = "30-day"
	vendorPrimary   = "primary_window"
	vendorSecondary = "secondary_window"
)

// Provider is the Codex implementation of provider.Provider plus
// provider.ConversationLister.
type Provider struct {
	UserHome string
	// Endpoint overrides DefaultEndpoint (tests).
	Endpoint string
	// HTTP is the transport; nil means a client with DefaultTimeout.
	HTTP *http.Client
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	// historyIndex caches, per home, the set of cwds that have a rollout.
	// In-memory for one process only (never persisted), so a gc run over
	// many folders walks the sessions tree once instead of once per folder.
	mu           sync.Mutex
	historyIndex map[string]map[string]bool
}

var (
	_ provider.Provider           = (*Provider)(nil)
	_ provider.ConversationLister = (*Provider)(nil)
	_ provider.HistoryChecker     = (*Provider)(nil)
	_ provider.MoveUnsupported    = (*Provider)(nil)
)

// HasHistory implements provider.HistoryChecker. Codex keys rollouts by id,
// so the first call per home builds an index of every rollout's cwd.
func (p *Provider) HasHistory(home, dir string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.historyIndex == nil {
		p.historyIndex = map[string]map[string]bool{}
	}
	idx, ok := p.historyIndex[home]
	if !ok {
		idx = map[string]bool{}
		convs, err := p.Conversations(context.Background(), home, provider.ConversationQuery{})
		if err == nil {
			for _, c := range convs {
				idx[filepath.Clean(c.Dir)] = true
			}
		}
		p.historyIndex[home] = idx
	}
	return idx[filepath.Clean(dir)]
}

// New returns a Provider with a bounded-timeout HTTP client.
func New(userHome string) *Provider {
	return &Provider{UserHome: userHome, HTTP: &http.Client{Timeout: DefaultTimeout}}
}

// ID implements provider.Provider.
func (p *Provider) ID() provider.ID { return provider.Codex }

// Binary implements provider.Provider.
func (p *Provider) Binary() string { return Binary }

// DefaultHome implements provider.Provider.
func (p *Provider) DefaultHome(userHome string) string {
	return filepath.Join(userHome, defaultDirName)
}

// Discover implements provider.Provider: ~/.codex with an auth.json becomes
// profile "codex".
func (p *Provider) Discover(userHome string) ([]provider.Profile, error) {
	home := p.DefaultHome(userHome)
	if _, err := os.Stat(filepath.Join(home, authFile)); err != nil {
		return nil, nil
	}
	a, err := readAuth(home)
	billing := provider.BillingPlan
	if err == nil && a.apiKeyOnly() {
		billing = provider.BillingAPI
	}
	return []provider.Profile{{
		Name: defaultProfileName, Provider: provider.Codex, Home: home,
		Billing: billing, Default: true, Source: provider.SourceDiscovered,
	}}, nil
}

// auth is the subset of <home>/auth.json agx reads. The refresh token and
// id token are deliberately not decoded.
type auth struct {
	AuthMode     string  `json:"auth_mode"`
	OpenAIAPIKey *string `json:"OPENAI_API_KEY"`
	Tokens       *struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	} `json:"tokens"`
}

// apiKeyOnly reports a login that is an API key with no ChatGPT tokens.
func (a auth) apiKeyOnly() bool {
	return (a.Tokens == nil || a.Tokens.AccessToken == "") && a.OpenAIAPIKey != nil && *a.OpenAIAPIKey != ""
}

// readAuth loads <home>/auth.json.
func readAuth(home string) (auth, error) {
	raw, err := os.ReadFile(filepath.Join(home, authFile))
	if err != nil {
		return auth{}, err
	}
	var a auth
	if err := json.Unmarshal(raw, &a); err != nil {
		return auth{}, fmt.Errorf("codex: decode %s: %w", authFile, err)
	}
	return a, nil
}

// Identity implements provider.Provider. Codex keeps no email locally, so
// Email / Plan are filled only by Usage (from the server response).
func (p *Provider) Identity(pr provider.Profile) (provider.Identity, error) {
	a, err := readAuth(pr.Home)
	if errors.Is(err, os.ErrNotExist) {
		return provider.Identity{}, nil
	}
	if err != nil {
		return provider.Identity{}, err
	}
	id := provider.Identity{CredentialSource: filepath.Join(pr.Home, authFile)}
	id.LoggedIn = (a.Tokens != nil && a.Tokens.AccessToken != "") || a.apiKeyOnly()
	return id, nil
}

// wireUsage is the subset of GET /backend-api/wham/usage agx reads.
type wireUsage struct {
	Email     string `json:"email"`
	PlanType  string `json:"plan_type"`
	RateLimit *struct {
		LimitReached    bool        `json:"limit_reached"`
		PrimaryWindow   *wireWindow `json:"primary_window"`
		SecondaryWindow *wireWindow `json:"secondary_window"`
	} `json:"rate_limit"`
}

// wireWindow is one Codex window. reset_at is Unix seconds; it can be absent.
type wireWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAt            *int64  `json:"reset_at"`
}

// ErrNoWindows means the response carried no rate-limit windows.
var ErrNoWindows = errors.New("codex: usage response has no rate-limit windows")

// ParseUsage decodes a usage response.
//
// Invariant: a nil error always comes with at least one window.
func ParseUsage(body []byte) (provider.Usage, error) {
	var w wireUsage
	if err := json.Unmarshal(body, &w); err != nil {
		return provider.Usage{}, fmt.Errorf("codex: decode usage: %w", err)
	}
	u := provider.Usage{Identity: provider.Identity{Email: w.Email, Plan: planLabel(w.PlanType), LoggedIn: true}}
	if w.RateLimit != nil {
		for _, pair := range []struct {
			win    *wireWindow
			vendor string
		}{{w.RateLimit.PrimaryWindow, vendorPrimary}, {w.RateLimit.SecondaryWindow, vendorSecondary}} {
			if pair.win == nil {
				continue
			}
			win := toWindow(*pair.win, pair.vendor)
			if w.RateLimit.LimitReached && win.Percent >= percentFull {
				win.Severity = provider.SeverityExhausted
			}
			u.Windows = append(u.Windows, win)
		}
	}
	if len(u.Windows) == 0 {
		return provider.Usage{}, ErrNoWindows
	}
	return u, nil
}

// toWindow maps a Codex window, labelling it by its length.
func toWindow(w wireWindow, vendor string) provider.Window {
	out := provider.Window{Group: provider.GroupPlan, Percent: w.UsedPercent, VendorKind: vendor}
	switch w.LimitWindowSeconds {
	case fiveHourSeconds:
		out.Kind, out.Label = provider.WindowSession, labelFiveHour
	case weekSeconds:
		out.Kind, out.Label = provider.WindowWeekly, labelWeekly
	case thirtyDaySeconds:
		out.Kind, out.Label = provider.WindowOther, labelThirtyDay
	default:
		out.Kind = provider.WindowOther
		out.Label = fmt.Sprintf("%d-day", max(1, w.LimitWindowSeconds/daySeconds))
	}
	if w.ResetAt != nil {
		t := time.Unix(*w.ResetAt, 0).UTC()
		out.ResetsAt = &t
	}
	return out
}

// planLabel title-cases Codex's plan_type ("plus" → "Plus").
func planLabel(plan string) string {
	if plan == "" {
		return ""
	}
	return strings.ToUpper(plan[:1]) + plan[1:]
}

// Usage implements provider.Provider.
func (p *Provider) Usage(ctx context.Context, pr provider.Profile) (provider.Usage, error) {
	if pr.Billing == provider.BillingAPI {
		return provider.Usage{}, provider.ErrAPIBilled
	}
	a, err := readAuth(pr.Home)
	if errors.Is(err, os.ErrNotExist) {
		return provider.Usage{}, provider.ErrNotLoggedIn
	}
	if err != nil {
		return provider.Usage{}, err
	}
	if a.apiKeyOnly() {
		return provider.Usage{}, provider.ErrAPIBilled
	}
	if a.Tokens == nil || a.Tokens.AccessToken == "" {
		return provider.Usage{}, provider.ErrNotLoggedIn
	}

	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return provider.Usage{}, fmt.Errorf("codex: build usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.Tokens.AccessToken)
	if a.Tokens.AccountID != "" {
		req.Header.Set(accountIDHeader, a.Tokens.AccountID)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	client := p.HTTP
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return provider.Usage{}, fmt.Errorf("codex: usage request: %w", err)
	}
	// The body is fully read (bounded) below; closing only releases the connection.
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return provider.Usage{}, fmt.Errorf("codex: read usage response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return provider.Usage{}, fmt.Errorf("%w (HTTP %d)", provider.ErrUnauthorized, resp.StatusCode)
	case resp.StatusCode == http.StatusTooManyRequests:
		return provider.Usage{}, provider.RateLimitError(resp.Header)
	case resp.StatusCode != http.StatusOK:
		b := strings.Join(strings.Fields(string(body)), " ") // one line, even for pretty JSON
		if len(b) > snippetMax {
			b = b[:snippetMax] + "…"
		}
		return provider.Usage{}, fmt.Errorf("codex: unexpected HTTP %d: %s", resp.StatusCode, b)
	}
	u, err := ParseUsage(body)
	u.Identity.CredentialSource = filepath.Join(pr.Home, authFile)
	return u, err
}

// Launch implements provider.Provider.
func (p *Provider) Launch(pr provider.Profile, req provider.LaunchRequest) (provider.Command, error) {
	var args []string
	if req.ResumeID != "" {
		args = append(args, resumeSubcommand)
	}
	args = append(args, pr.Args...)
	if req.ResumeID != "" {
		args = append(args, req.ResumeID)
	}
	args = append(args, req.Args...)

	env := provider.HomeEnv(req.Env, HomeEnv, pr.Home, p.DefaultHome(p.UserHome))
	for k, v := range pr.Env {
		env = provider.SetEnv(env, k, v)
	}
	return provider.Command{Path: Binary, Args: args, Env: env, Dir: req.Dir}, nil
}

// sessionMeta is the first record of a rollout file.
type sessionMeta struct {
	Type    string `json:"type"`
	Payload struct {
		ID  string `json:"id"`
		Cwd string `json:"cwd"`
	} `json:"payload"`
}

// Conversations implements provider.ConversationLister. Codex rollouts carry
// no title, so Title stays empty. Only the first line of each file is read.
func (p *Provider) Conversations(ctx context.Context, home string, q provider.ConversationQuery) ([]provider.Conversation, error) {
	root := filepath.Join(home, sessionsDir)
	type cand struct {
		path string
		mod  time.Time
	}
	var cands []cand
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return filepath.SkipDir
			}
			return nil // unreadable subtree: skip, don't fail the listing
		}
		if d.IsDir() || !strings.HasPrefix(d.Name(), rolloutPrefix) || !strings.HasSuffix(d.Name(), transcriptExt) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		cands = append(cands, cand{path: path, mod: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("codex: walk sessions: %w", err)
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod.After(cands[j].mod) })

	var out []provider.Conversation
	for _, c := range cands {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
		meta, ok := readMeta(c.path)
		if !ok {
			continue
		}
		if q.Dir != "" && filepath.Clean(meta.Payload.Cwd) != filepath.Clean(q.Dir) {
			continue
		}
		out = append(out, provider.Conversation{
			Provider: provider.Codex, Home: home, ID: meta.Payload.ID,
			Dir: meta.Payload.Cwd, Updated: c.mod,
		})
	}
	return out, nil
}

// readMeta reads a rollout's first line.
func readMeta(path string) (sessionMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return sessionMeta{}, false
	}
	// Read-only: a close error cannot lose data, and the read already succeeded or failed.
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(io.LimitReader(f, headScanBytes))
	sc.Buffer(make([]byte, 0, 64<<10), headScanBytes)
	if !sc.Scan() {
		return sessionMeta{}, false
	}
	var m sessionMeta
	if json.Unmarshal(sc.Bytes(), &m) != nil || m.Type != recordSessionMeta || m.Payload.ID == "" || m.Payload.Cwd == "" {
		return sessionMeta{}, false
	}
	return m, true
}

// MoveUnsupportedReason implements provider.MoveUnsupported: Codex indexes
// conversations in SQLite (state_5.sqlite threads.rollout_path holds absolute
// paths; titles live in session_index.jsonl), so moving rollout files would
// leave its index pointing at files that are gone.
func (p *Provider) MoveUnsupportedReason() string {
	return "Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite), so moving its files would break its index; continue Codex work in the original account"
}
