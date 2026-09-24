package account

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Profile is the human identity of an account, used only for display.
type Profile struct {
	Email        string
	Organization string
}

// LoadProfile reads the account's email and organization.
//
// Where the file lives differs by dir: a CLAUDE_CONFIG_DIR dir keeps it at
// <dir>/.claude.json, the default dir at ~/.claude.json. Both are tried for
// the default dir (dir-local first). A missing or unreadable file yields an
// empty Profile rather than an error: identity is cosmetic, and the report
// falls back to showing the config dir.
func LoadProfile(dir, home string, isDefault bool) Profile {
	paths := []string{filepath.Join(dir, profileFile)}
	if isDefault {
		paths = append(paths, filepath.Join(home, profileFile))
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var w struct {
			OAuthAccount struct {
				EmailAddress     string `json:"emailAddress"`
				OrganizationName string `json:"organizationName"`
			} `json:"oauthAccount"`
		}
		if json.Unmarshal(raw, &w) != nil || w.OAuthAccount.EmailAddress == "" {
			continue
		}
		return Profile{Email: w.OAuthAccount.EmailAddress, Organization: w.OAuthAccount.OrganizationName}
	}
	return Profile{}
}
