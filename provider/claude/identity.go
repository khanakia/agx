package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Account is the human identity of a login, used only for display.
type Account struct {
	Email        string
	Organization string
}

// LoadAccount reads the account's email and organization.
//
// Where the file lives differs by dir: a CLAUDE_CONFIG_DIR dir keeps it at
// <dir>/.claude.json, the default dir at ~/.claude.json. Both are tried for
// the default dir (dir-local first). A missing or unreadable file yields an
// empty Account rather than an error: identity is cosmetic, and callers fall
// back to showing the config dir.
func LoadAccount(home, userHome string, isDefault bool) Account {
	paths := []string{filepath.Join(home, profileFile)}
	if isDefault {
		paths = append(paths, filepath.Join(userHome, profileFile))
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
		return Account{Email: w.OAuthAccount.EmailAddress, Organization: w.OAuthAccount.OrganizationName}
	}
	return Account{}
}
