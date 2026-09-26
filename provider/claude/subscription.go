package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/khanakia/agx/provider"
)

var _ provider.SubscriptionReader = (*Provider)(nil)

// orgTypePrefix is stripped from organization_type ("claude_max" → "max")
// to reuse PlanLabel's formatting.
const orgTypePrefix = "claude_"

// ErrNoOrganization means the profile response carried no organization.
var ErrNoOrganization = errors.New("claude: profile response has no organization")

// wireProfile is the subset of GET /api/oauth/profile agx reads. Personal
// fields (names, email, uuids) are deliberately not decoded.
type wireProfile struct {
	Organization *struct {
		OrganizationType      string     `json:"organization_type"`
		RateLimitTier         string     `json:"rate_limit_tier"`
		BillingType           string     `json:"billing_type"`
		SubscriptionStatus    string     `json:"subscription_status"`
		SubscriptionCreatedAt *time.Time `json:"subscription_created_at"`
	} `json:"organization"`
}

// ParseProfile decodes a profile response into a Subscription.
func ParseProfile(body []byte) (provider.Subscription, error) {
	var w wireProfile
	if err := json.Unmarshal(body, &w); err != nil {
		return provider.Subscription{}, fmt.Errorf("claude: decode profile: %w", err)
	}
	if w.Organization == nil {
		return provider.Subscription{}, ErrNoOrganization
	}
	o := w.Organization
	return provider.Subscription{
		Plan:      PlanLabel(strings.TrimPrefix(o.OrganizationType, orgTypePrefix), o.RateLimitTier),
		Status:    o.SubscriptionStatus,
		Billing:   o.BillingType,
		StartedAt: o.SubscriptionCreatedAt,
	}, nil
}

// Subscription implements provider.SubscriptionReader with the same login
// resolution (and read-only guarantees) as Usage.
func (p *Provider) Subscription(ctx context.Context, pr provider.Profile) (provider.Subscription, error) {
	if pr.Billing == provider.BillingAPI {
		return provider.Subscription{}, provider.ErrAPIBilled
	}
	id, err := p.Identity(pr)
	if err != nil {
		return provider.Subscription{}, err
	}
	if !id.LoggedIn {
		return provider.Subscription{}, provider.ErrNotLoggedIn
	}
	if !id.ExpiresAt.IsZero() && !p.now().Before(id.ExpiresAt) {
		return provider.Subscription{}, fmt.Errorf("%w %s", provider.ErrExpired, id.ExpiresAt.Local().Format(time.DateTime))
	}
	cred, err := ResolveCredential(pr.Home, IsDefaultHome(pr.Home, p.UserHome), p.Store)
	if err != nil {
		return provider.Subscription{}, err
	}
	client := p.UsageClient
	if client == nil {
		client = &UsageClient{}
	}
	return client.FetchProfile(ctx, cred.AccessToken)
}
