package claude

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/khanakia/agx/provider"
)

// UsageClient fetches usage snapshots for an access token.
//
// It is read-only by design: it never refreshes a token (see package doc).
// A rejected token is reported as provider.ErrUnauthorized.
type UsageClient struct {
	// Endpoint is the usage URL; empty means DefaultEndpoint. Tests point it
	// at an httptest server.
	Endpoint string
	// HTTP is the transport; nil means a client with DefaultTimeout.
	HTTP *http.Client
	// ProfileEndpoint overrides DefaultProfileEndpoint (tests).
	ProfileEndpoint string
}

// Fetch returns the usage snapshot for the account owning accessToken. The
// call is a GET and does not consume plan quota.
func (c *UsageClient) Fetch(ctx context.Context, accessToken string) (provider.Usage, error) {
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	body, err := c.get(ctx, endpoint, accessToken)
	if err != nil {
		return provider.Usage{}, err
	}
	return ParseUsage(body)
}

// FetchProfile returns the account profile (plan, subscription status and
// start) for accessToken — GET /api/oauth/profile, read-only.
func (c *UsageClient) FetchProfile(ctx context.Context, accessToken string) (provider.Subscription, error) {
	endpoint := c.ProfileEndpoint
	if endpoint == "" {
		endpoint = DefaultProfileEndpoint
	}
	body, err := c.get(ctx, endpoint, accessToken)
	if err != nil {
		return provider.Subscription{}, err
	}
	return ParseProfile(body)
}

// get performs one authenticated GET and maps HTTP failures onto the
// provider sentinels (401/403 → ErrUnauthorized, 429 → ErrRateLimited).
func (c *UsageClient) get(ctx context.Context, endpoint, accessToken string) ([]byte, error) {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("claude: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set(oauthBetaHeader, oauthBetaValue)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude: request %s: %w", endpoint, err)
	}
	// The body is fully read (bounded) below; closing only releases the connection.
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("claude: read response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w (HTTP %d)", provider.ErrUnauthorized, resp.StatusCode)
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, provider.RateLimitError(resp.Header)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("claude: unexpected HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	return body, nil
}

// snippet trims an error body for inclusion in an error message.
func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ") // one line, even for pretty JSON
	if len(s) > snippetMax {
		return s[:snippetMax] + "…"
	}
	return s
}
