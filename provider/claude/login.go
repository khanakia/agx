package claude

import (
	"time"

	"github.com/khanakia/agx/provider"
)

// Env vars Claude Code reads a credential from (logins spec §7). They are
// listed together because a login of one kind must remove the others: an
// API key or auth token left in the environment would otherwise take
// precedence and silently bill the wrong way.
const (
	// OAuthTokenEnv carries a long-lived `claude setup-token` token.
	OAuthTokenEnv = "CLAUDE_CODE_OAUTH_TOKEN"
	// APIKeyEnv carries a console API key (pay per token).
	APIKeyEnv = "ANTHROPIC_API_KEY"
	// AuthTokenEnv is the bearer token third-party backends use (kimi).
	AuthTokenEnv = "ANTHROPIC_AUTH_TOKEN"
	// SetupTokenSubcommand is the `claude` subcommand that mints an
	// oauth-token login (`agx login add --setup` runs it).
	SetupTokenSubcommand = "setup-token"
)

// SetupTokenLifetime is how long a setup-token is believed to stay valid,
// counted from the login's `added` date. Unverified (logins spec §12 V3).
// agx shows it as EXPIRES, `agx doctor` warns 30 days before it, and a run
// on the login is refused after it (logins spec §9) with the renew command.
const SetupTokenLifetime = 365 * 24 * time.Hour

// Compile-time checks that the login upgrades stay implemented.
var (
	_ provider.LoginLauncher  = (*Provider)(nil)
	_ provider.LoginLifetimer = (*Provider)(nil)
)

// LoginKinds implements provider.LoginLauncher; oauth-token first, so a
// plain `agx login add` spends plan quota rather than an API key.
func (p *Provider) LoginKinds() []provider.LoginKind {
	return []provider.LoginKind{provider.LoginOAuthToken, provider.LoginAPIKey}
}

// LoginEnv implements provider.LoginLauncher. An unknown kind sets nothing,
// which callers have already refused at config validation.
func (p *Provider) LoginEnv(kind provider.LoginKind, value string) (map[string]string, []string) {
	switch kind {
	case provider.LoginOAuthToken:
		return map[string]string{OAuthTokenEnv: value}, []string{APIKeyEnv, AuthTokenEnv}
	case provider.LoginAPIKey:
		return map[string]string{APIKeyEnv: value}, []string{OAuthTokenEnv, AuthTokenEnv}
	}
	return nil, nil
}

// LoginLifetime implements provider.LoginLifetimer: setup-tokens expire,
// API keys live until revoked.
func (p *Provider) LoginLifetime(kind provider.LoginKind) time.Duration {
	if kind == provider.LoginOAuthToken {
		return SetupTokenLifetime
	}
	return 0
}
