package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/khanakia/agx/provider"
)

// The usage probe (logins spec §14 D14). A setup-token cannot read
// /api/oauth/usage (403, scope user:profile), but every inference response
// carries the account's unified rate-limit state in its headers — the same
// windows /usage shows. So a token login's usage costs one minimal
// /v1/messages call. It spends a few tokens of plan quota and counts toward
// rate limits, which is why callers run it only when the user asked
// (`agx usage --logins`, `--login NAME`, `--login auto`, `agx login check`).
const (
	// DefaultProbeEndpoint is the Messages API; tests point ProbeClient at
	// an httptest server instead.
	DefaultProbeEndpoint = "https://api.anthropic.com/v1/messages"
	// DefaultProbeModel is the smallest current model: the probe only needs
	// the response headers, so the cheapest model keeps its cost to a few
	// tokens. Override with ProbeClient.Model if it is retired.
	DefaultProbeModel = "claude-haiku-5-5"
	// DefaultProbeTimeout bounds one probe. Longer than DefaultTimeout
	// because it waits for a (one-token) model response, not a stored row.
	DefaultProbeTimeout = 30 * time.Second
	// probeMaxTokens is the smallest completion the API accepts.
	probeMaxTokens = 1
	// probeSystem is the system prompt Claude Code sends; the API accepts
	// a setup-token only for requests that identify as Claude Code.
	probeSystem = "You are Claude Code, Anthropic's official CLI for Claude."
	// probeRole / probeUserText are the shortest valid conversation: one
	// user turn.
	probeRole     = "user"
	probeUserText = "hi"
	// anthropicVersionHeader / anthropicVersion pin the Messages API
	// version the request body is written against.
	anthropicVersionHeader = "anthropic-version"
	anthropicVersion       = "2023-06-01"
	// redactedToken replaces the token wherever an error body repeats it.
	redactedToken = "<redacted>"
)

// Response headers the probe reads (verified on the real API 2026-10-16).
// Utilization is a fraction 0..1; reset is unix seconds.
const (
	hdrPrefix         = "anthropic-ratelimit-unified-"
	hdrUtilization    = "-utilization"
	hdrReset          = "-reset"
	hdrStatus         = "-status"
	hdrRepresentative = hdrPrefix + "representative-claim"
	// probeWindow5h / probeWindow7d are the header infixes; the same names
	// are the representative-claim values that mark the binding window.
	probeWindow5h = "5h"
	probeWindow7d = "7d"
	claim5h       = "five_hour"
	claim7d       = "seven_day"
)

// Unified status values, mapped onto the usage endpoint's severities so a
// probed window colours like a fetched one.
const (
	statusAllowed        = "allowed"
	statusAllowedWarning = "allowed_warning"
	// severityExhausted is what "rejected" (or any other non-empty status)
	// becomes: severity() already treats an unknown wire severity as
	// exhausted, so this value keeps a probed window colouring the same way.
	severityExhausted = "exhausted"
)

// percentPerFraction converts utilization (0..1) to the percent the window
// model uses (0..100).
const percentPerFraction = 100

// ErrProbeHeaders means a probe answered 200 without the rate-limit headers
// it is read for. It is an error, never a 0% window, so an upstream change
// is not mistaken for an idle account.
var ErrProbeHeaders = errors.New("claude: probe response has no usable rate-limit headers")

// probeWindows lists the windows a probe reads, in render order, with the
// wire kind/group the usage endpoint uses for the same window.
var probeWindows = []struct {
	infix, kind, group, claim string
}{
	{probeWindow5h, kindSession, groupSession, claim5h},
	{probeWindow7d, kindWeeklyAll, groupWeekly, claim7d},
}

// ProbeClient reads a token's usage from one minimal Messages call.
type ProbeClient struct {
	// Endpoint is the Messages URL; empty means DefaultProbeEndpoint.
	Endpoint string
	// Model is the model probed; empty means DefaultProbeModel.
	Model string
	// HTTP is the transport; nil means a client with DefaultProbeTimeout.
	HTTP *http.Client
}

// probeBody is the request body (only the fields the probe needs).
type probeBody struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	System    string         `json:"system"`
	Messages  []probeMessage `json:"messages"`
}

type probeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Probe sends one minimal request with token and maps the response headers
// onto usage windows. 401/403 → provider.ErrUnauthorized, 429 →
// provider.RateLimitError; the token never appears in an error.
func (c *ProbeClient) Probe(ctx context.Context, token string) (provider.Usage, error) {
	endpoint, model, httpClient := c.Endpoint, c.Model, c.HTTP
	if endpoint == "" {
		endpoint = DefaultProbeEndpoint
	}
	if model == "" {
		model = DefaultProbeModel
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultProbeTimeout}
	}
	raw, err := json.Marshal(probeBody{
		Model: model, MaxTokens: probeMaxTokens, System: probeSystem,
		Messages: []probeMessage{{Role: probeRole, Content: probeUserText}},
	})
	if err != nil {
		return provider.Usage{}, fmt.Errorf("claude: encode probe: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return provider.Usage{}, fmt.Errorf("claude: build probe: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(oauthBetaHeader, oauthBetaValue)
	req.Header.Set(anthropicVersionHeader, anthropicVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		// *url.Error quotes the URL only; the token is in a header.
		return provider.Usage{}, fmt.Errorf("claude: probe %s: %w", endpoint, err)
	}
	// The body is read (bounded) below; closing only releases the connection.
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return provider.Usage{}, fmt.Errorf("claude: read probe response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return provider.Usage{}, fmt.Errorf("%w (HTTP %d%s)", provider.ErrUnauthorized, resp.StatusCode, apiMessage(body, token))
	case resp.StatusCode == http.StatusTooManyRequests:
		return provider.Usage{}, provider.RateLimitError(resp.Header)
	case resp.StatusCode != http.StatusOK:
		return provider.Usage{}, fmt.Errorf("claude: probe: unexpected HTTP %d: %s", resp.StatusCode, snippet([]byte(redact(string(body), token))))
	}
	return ParseProbeHeaders(resp.Header)
}

// apiMessage returns ": <error.message>" from an API error body, redacted,
// or "" when the body carries none.
func apiMessage(body []byte, token string) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || e.Error.Message == "" {
		return ""
	}
	return ": " + snippet([]byte(redact(e.Error.Message, token)))
}

// redact removes token from s. An empty token is left alone (ReplaceAll
// would otherwise insert the mark between every byte).
func redact(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, redactedToken)
}

// ParseProbeHeaders maps the anthropic-ratelimit-unified-* headers onto the
// same windows the usage endpoint yields (session "Current session", weekly
// "All models"). Utilization and reset are required for every window: a
// missing or malformed one is an error naming the header.
//
// Invariant: a nil error always comes with one window per probeWindows entry.
func ParseProbeHeaders(h http.Header) (provider.Usage, error) {
	claim := h.Get(hdrRepresentative)
	u := provider.Usage{Windows: make([]provider.Window, 0, len(probeWindows))}
	for _, pw := range probeWindows {
		base := hdrPrefix + pw.infix
		utilRaw, resetRaw := h.Get(base+hdrUtilization), h.Get(base+hdrReset)
		if utilRaw == "" {
			return provider.Usage{}, fmt.Errorf("%w: %s missing", ErrProbeHeaders, base+hdrUtilization)
		}
		if resetRaw == "" {
			return provider.Usage{}, fmt.Errorf("%w: %s missing", ErrProbeHeaders, base+hdrReset)
		}
		util, err := strconv.ParseFloat(utilRaw, 64)
		if err != nil || util < 0 {
			return provider.Usage{}, fmt.Errorf("%w: %s=%q is not a fraction", ErrProbeHeaders, base+hdrUtilization, utilRaw)
		}
		secs, err := strconv.ParseInt(resetRaw, 10, 64)
		if err != nil || secs <= 0 {
			return provider.Usage{}, fmt.Errorf("%w: %s=%q is not a unix time", ErrProbeHeaders, base+hdrReset, resetRaw)
		}
		reset := time.Unix(secs, 0).UTC()
		u.Windows = append(u.Windows, toWindow(Limit{
			Kind: pw.kind, Group: pw.group, Percent: util * percentPerFraction,
			Severity: probeSeverity(h.Get(base + hdrStatus)), ResetsAt: &reset,
			IsActive: claim == pw.claim,
		}))
	}
	return u, nil
}

// probeSeverity maps a unified status onto the usage endpoint's severity
// wire values; "" stays "" (unknown), anything unrecognised is exhausted.
func probeSeverity(status string) string {
	switch status {
	case "":
		return ""
	case statusAllowed:
		return severityNormal
	case statusAllowedWarning:
		return severityWarning
	default:
		return severityExhausted
	}
}
