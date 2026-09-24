package usage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrUnauthorized means the server rejected the access token (401/403) —
// almost always because it expired and Claude Code has not refreshed it yet.
var ErrUnauthorized = errors.New("usage: access token rejected")

// Client fetches usage snapshots.
//
// It is read-only by design: it never refreshes a token. Refreshing rotates
// the refresh token, which would silently log the owning Claude Code install
// out, so an expired token is reported instead (ErrUnauthorized).
type Client struct {
	// Endpoint is the usage URL; empty means DefaultEndpoint. Tests point it
	// at an httptest server.
	Endpoint string
	// HTTP is the transport; nil means a client with DefaultTimeout.
	HTTP *http.Client
}

// NewClient returns a Client for DefaultEndpoint with a bounded timeout.
func NewClient() *Client {
	return &Client{Endpoint: DefaultEndpoint, HTTP: &http.Client{Timeout: DefaultTimeout}}
}

// Fetch returns the usage snapshot for the account owning accessToken.
//
// The call is a GET and does not consume plan quota.
func (c *Client) Fetch(ctx context.Context, accessToken string) (Usage, error) {
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Usage{}, fmt.Errorf("usage: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set(oauthBetaHeader, oauthBetaValue)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return Usage{}, fmt.Errorf("usage: request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Usage{}, fmt.Errorf("usage: read response: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Usage{}, fmt.Errorf("%w (HTTP %d)", ErrUnauthorized, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return Usage{}, fmt.Errorf("usage: unexpected HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	return Parse(body)
}

// snippetMax bounds how much of an error body is echoed into a message.
const snippetMax = 200

// snippet trims an error body for inclusion in an error message.
func snippet(b []byte) string {
	if len(b) > snippetMax {
		return string(b[:snippetMax]) + "…"
	}
	return string(b)
}
