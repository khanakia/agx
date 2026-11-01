package codex

import "github.com/khanakia/agx/provider"

// loginSubcommand starts Codex's interactive login, which writes auth.json in
// the home CODEX_HOME selects.
const loginSubcommand = "login"

// Compile-time check that the home upgrade stays implemented.
var _ provider.HomeSetup = (*Provider)(nil)

// SharedHomeFiles implements provider.HomeSetup: nothing is shared, a Codex
// home has no file agx has checked as safe to link.
func (p *Provider) SharedHomeFiles() []string { return nil }

// LoginCommand implements provider.HomeSetup.
func (p *Provider) LoginCommand(pr provider.Profile, env []string) provider.Command {
	env = provider.HomeEnv(env, HomeEnv, pr.Home, p.DefaultHome(p.UserHome))
	return provider.Command{Path: Binary, Args: []string{loginSubcommand}, Env: env}
}
