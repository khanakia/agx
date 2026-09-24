package claude

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/khanakia/agx/provider"
)

// Provider is the Claude Code implementation of provider.Provider, plus the
// ConversationLister and HistoryMover upgrades.
type Provider struct {
	// UserHome is the user's home dir (for ~/.claude and ~/.claude.json).
	UserHome string
	// Store reads keychain entries; nil disables the keychain (file only).
	Store SecretStore
	// Usage fetches usage; nil means a default UsageClient.
	UsageClient *UsageClient
	// Now is the clock for expiry checks; nil means time.Now.
	Now func() time.Time
}

// Compile-time checks that the optional upgrades stay implemented.
var (
	_ provider.Provider           = (*Provider)(nil)
	_ provider.ConversationLister = (*Provider)(nil)
	_ provider.HistoryMover       = (*Provider)(nil)
	_ provider.HistoryChecker     = (*Provider)(nil)
)

// New returns a Provider that reads the macOS keychain and fetches usage with
// a bounded-timeout client.
func New(userHome string) *Provider {
	return &Provider{UserHome: userHome, Store: Keychain{}, UsageClient: &UsageClient{}}
}

// ID implements provider.Provider.
func (p *Provider) ID() provider.ID { return provider.Claude }

// Binary implements provider.Provider.
func (p *Provider) Binary() string { return Binary }

// DefaultHome implements provider.Provider.
func (p *Provider) DefaultHome(userHome string) string { return DefaultHomeFor(userHome) }

// Discover implements provider.Provider: one plan-billed profile per config
// dir found on disk, ~/.claude being the default.
func (p *Provider) Discover(userHome string) ([]provider.Profile, error) {
	homes, err := DiscoverHomes(userHome)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Profile, 0, len(homes))
	for _, h := range homes {
		out = append(out, provider.Profile{
			Name: h.Name, Provider: provider.Claude, Home: h.Home,
			Billing: provider.BillingPlan, Default: IsDefaultHome(h.Home, userHome),
			Source: provider.SourceDiscovered,
		})
	}
	return out, nil
}

// Identity implements provider.Provider without touching the network.
func (p *Provider) Identity(pr provider.Profile) (provider.Identity, error) {
	isDefault := IsDefaultHome(pr.Home, p.UserHome)
	acct := LoadAccount(pr.Home, p.UserHome, isDefault)
	id := provider.Identity{Email: acct.Email, Organization: acct.Organization}

	cred, err := ResolveCredential(pr.Home, isDefault, p.Store)
	if errors.Is(err, ErrNoCredential) {
		return id, nil // not logged in is a state, not a failure
	}
	if err != nil {
		return id, err
	}
	id.LoggedIn = true
	id.ExpiresAt = cred.ExpiresAt
	id.Plan = PlanLabel(cred.SubscriptionType, cred.RateLimitTier)
	id.CredentialSource = cred.Source
	return id, nil
}

// Usage implements provider.Provider.
func (p *Provider) Usage(ctx context.Context, pr provider.Profile) (provider.Usage, error) {
	if pr.Billing == provider.BillingAPI {
		return provider.Usage{}, provider.ErrAPIBilled
	}
	isDefault := IsDefaultHome(pr.Home, p.UserHome)
	id, err := p.Identity(pr)
	if err != nil {
		return provider.Usage{Identity: id}, err
	}
	if !id.LoggedIn {
		return provider.Usage{Identity: id}, provider.ErrNotLoggedIn
	}
	if !id.ExpiresAt.IsZero() && !p.now().Before(id.ExpiresAt) {
		return provider.Usage{Identity: id}, fmt.Errorf("%w %s", provider.ErrExpired, id.ExpiresAt.Local().Format(time.DateTime))
	}
	cred, err := ResolveCredential(pr.Home, isDefault, p.Store)
	if err != nil {
		return provider.Usage{Identity: id}, err
	}
	client := p.UsageClient
	if client == nil {
		client = &UsageClient{}
	}
	u, err := client.Fetch(ctx, cred.AccessToken)
	u.Identity = id
	return u, err
}

// Launch implements provider.Provider. CLAUDE_CONFIG_DIR is set for
// non-default homes and removed for the default one (see provider.HomeEnv
// for why removal matters).
func (p *Provider) Launch(pr provider.Profile, req provider.LaunchRequest) (provider.Command, error) {
	args := append([]string{}, pr.Args...)
	if req.ResumeID != "" {
		args = append(args, resumeFlag, req.ResumeID)
	}
	args = append(args, req.Args...)

	env := provider.HomeEnv(req.Env, ConfigDirEnv, pr.Home, DefaultHomeFor(p.UserHome))
	for k, v := range pr.Env {
		env = provider.SetEnv(env, k, v)
	}
	return provider.Command{Path: Binary, Args: args, Env: env, Dir: req.Dir}, nil
}

func (p *Provider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}
