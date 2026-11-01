package claude

import (
	"slices"

	"github.com/khanakia/agx/provider"
)

// sharedHomeFiles are what a new home links from ~/.claude: the user's own
// instructions, skills and settings. Credentials, history and caches are
// per-account and stay out.
var sharedHomeFiles = []string{"CLAUDE.md", "skills", "settings.json"}

// loginArgs start Claude Code's interactive login, which writes the login of
// the home CLAUDE_CONFIG_DIR selects.
var loginArgs = []string{"auth", "login"}

// credentialEnv are the variables that take precedence over a home's stored
// login. They are removed for the login command, or the run after it would
// keep using the shell's credential and the new login would look unused.
var credentialEnv = []string{OAuthTokenEnv, APIKeyEnv, AuthTokenEnv}

// Compile-time check that the home upgrade stays implemented.
var _ provider.HomeSetup = (*Provider)(nil)

// SharedHomeFiles implements provider.HomeSetup.
func (p *Provider) SharedHomeFiles() []string { return slices.Clone(sharedHomeFiles) }

// LoginCommand implements provider.HomeSetup.
func (p *Provider) LoginCommand(pr provider.Profile, env []string) provider.Command {
	env = provider.HomeEnv(env, ConfigDirEnv, pr.Home, DefaultHomeFor(p.UserHome))
	for _, k := range credentialEnv {
		env = provider.UnsetEnv(env, k)
	}
	return provider.Command{Path: Binary, Args: slices.Clone(loginArgs), Env: env}
}
