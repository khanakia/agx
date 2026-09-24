package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrNoCredential means no usable access token was found for a config dir —
// the account is not logged in (or was logged in with an API key, which has
// no plan limits to report).
var ErrNoCredential = errors.New("claude: no OAuth credential found")

// Credential is the OAuth login Claude Code stored for one config dir.
type Credential struct {
	AccessToken string
	// ExpiresAt is the zero time when the stored expiresAt is 0 or missing
	// (observed on stale pre-2.1.52 keychain entries); such a credential is
	// ranked below any credential with a known expiry.
	ExpiresAt time.Time
	// SubscriptionType ("max", "pro", …) and RateLimitTier
	// ("default_claude_max_20x", …) feed usage.PlanLabel.
	SubscriptionType string
	RateLimitTier    string
	// Source is SourceFile or SourceKeychain, plus the path or service name.
	Source string
}

// Expired reports whether the token has a known expiry at or before now.
// A credential with unknown expiry is never reported expired; the server's
// 401 is the authority for it.
func (c Credential) Expired(now time.Time) bool {
	return !c.ExpiresAt.IsZero() && !now.Before(c.ExpiresAt)
}

// SecretStore reads a secret by service name. It exists so tests (and
// non-macOS ports) can replace the macOS keychain.
type SecretStore interface {
	// Get returns the secret, or an error wrapping ErrSecretNotFound when no
	// entry exists for service.
	Get(service string) ([]byte, error)
}

// ErrSecretNotFound is returned by a SecretStore for a missing entry.
var ErrSecretNotFound = errors.New("claude: secret not found")

// KeychainServices returns the keychain service names that may hold dir's
// token, most specific first: the hashed name, then — for the default dir
// only — the bare legacy name.
//
// dir must be the same absolute, cleaned path Claude Code was given in
// CLAUDE_CONFIG_DIR, because the hash is over that exact string.
func KeychainServices(dir string, isDefault bool) []string {
	sum := sha256.Sum256([]byte(filepath.Clean(dir)))
	names := []string{keychainService + "-" + hex.EncodeToString(sum[:])[:keychainHashLen]}
	if isDefault {
		names = append(names, keychainService)
	}
	return names
}

// ResolveCredential finds the freshest OAuth token for dir across the
// credentials file and the keychain.
//
// Why "freshest": Claude Code may leave an old copy in one store after
// writing a refreshed token to the other (seen here: a keychain entry with
// expiresAt 0 beside a current .credentials.json). The candidate with the
// latest ExpiresAt wins; ties keep the earlier source (file, then keychain in
// KeychainServices order).
//
// Invariant: returns ErrNoCredential (wrapped, with per-source reasons) when
// nothing usable exists — never an empty Credential with a nil error.
func ResolveCredential(dir string, isDefault bool, store SecretStore) (Credential, error) {
	var (
		best    Credential
		found   bool
		reasons []error
	)
	consider := func(c Credential) {
		if !found || c.ExpiresAt.After(best.ExpiresAt) {
			best, found = c, true
		}
	}

	path := filepath.Join(dir, credentialsFile)
	switch raw, err := os.ReadFile(path); {
	case err == nil:
		if c, perr := parseCredential(raw, SourceFile+" "+path); perr == nil {
			consider(c)
		} else {
			reasons = append(reasons, perr)
		}
	case !errors.Is(err, os.ErrNotExist):
		reasons = append(reasons, fmt.Errorf("read %s: %w", path, err))
	}

	if store != nil {
		for _, svc := range KeychainServices(dir, isDefault) {
			raw, err := store.Get(svc)
			if errors.Is(err, ErrSecretNotFound) {
				continue
			}
			if err != nil {
				reasons = append(reasons, fmt.Errorf("keychain %q: %w", svc, err))
				continue
			}
			if c, perr := parseCredential(raw, SourceKeychain+" "+svc); perr == nil {
				consider(c)
			} else {
				reasons = append(reasons, perr)
			}
		}
	}

	if !found {
		return Credential{}, errors.Join(append([]error{ErrNoCredential}, reasons...)...)
	}
	return best, nil
}

// wireCredentials is the JSON Claude Code stores in both the credentials
// file and the keychain. Other top-level keys (mcpOAuth, …) and the refresh
// token are deliberately not decoded: this tool never needs them.
type wireCredentials struct {
	ClaudeAIOAuth *struct {
		AccessToken      string `json:"accessToken"`
		ExpiresAt        int64  `json:"expiresAt"` // Unix milliseconds; 0 = unknown
		SubscriptionType string `json:"subscriptionType"`
		RateLimitTier    string `json:"rateLimitTier"`
	} `json:"claudeAiOauth"`
}

// parseCredential decodes one stored credential blob.
func parseCredential(raw []byte, source string) (Credential, error) {
	var w wireCredentials
	if err := json.Unmarshal(raw, &w); err != nil {
		return Credential{}, fmt.Errorf("%s: decode: %w", source, err)
	}
	if w.ClaudeAIOAuth == nil || w.ClaudeAIOAuth.AccessToken == "" {
		return Credential{}, fmt.Errorf("%s: no claudeAiOauth.accessToken", source)
	}
	o := w.ClaudeAIOAuth
	c := Credential{
		AccessToken:      o.AccessToken,
		SubscriptionType: o.SubscriptionType,
		RateLimitTier:    o.RateLimitTier,
		Source:           source,
	}
	if o.ExpiresAt > 0 {
		c.ExpiresAt = time.UnixMilli(o.ExpiresAt).UTC()
	}
	return c, nil
}
