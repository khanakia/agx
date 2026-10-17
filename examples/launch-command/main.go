// Command launch-command builds the commands agx would exec for Claude Code
// and Codex, and prints them instead of running them. It shows the rule
// every wrapper must copy: the default home runs with CLAUDE_CONFIG_DIR /
// CODEX_HOME removed (not set to the default), a non-default home sets it,
// and a token login adds its variable while removing the ones that would
// bill a different way.
package main

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
	"github.com/khanakia/agx/provider/codex"
)

// userHome is a placeholder: Launch only builds paths, it reads nothing.
const userHome = "/home/demo"

// fakeLoginToken stands in for a setup-token; it is printed redacted.
const fakeLoginToken = "fake-setup-token"

// baseEnv is a shell that still exports CLAUDE_CONFIG_DIR from an earlier
// session, plus an API key that a plan login must not inherit.
var baseEnv = []string{
	"PATH=/usr/bin:/bin",
	"CLAUDE_CONFIG_DIR=/home/demo/.claude-old",
	"ANTHROPIC_API_KEY=fake-api-key",
}

// shownVars are the variables worth printing; PATH is noise here.
var shownVars = []string{claude.ConfigDirEnv, claude.OAuthTokenEnv, claude.APIKeyEnv, codex.HomeEnv}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cl := claude.New(userHome) // Launch never touches the keychain
	personal := provider.Profile{Name: "personal", Provider: provider.Claude, Home: cl.DefaultHome(userHome)}
	work := provider.Profile{
		Name: "work", Provider: provider.Claude, Home: filepath.Join(userHome, ".claude-work"),
		Args: []string{"--model", "opus"},
	}

	cmd, err := cl.Launch(personal, provider.LaunchRequest{Dir: "/srv/demo/api", Env: baseEnv})
	if err != nil {
		return err
	}
	show("personal (default home)", cmd)

	cmd, err = cl.Launch(work, provider.LaunchRequest{Env: baseEnv, ResumeID: "a1", Args: []string{"--verbose"}})
	if err != nil {
		return err
	}
	show("work, resuming a1", cmd)

	// A token login on top of the work home: set the token, remove the
	// variables that would otherwise take precedence and bill differently.
	set, unset := cl.LoginEnv(provider.LoginOAuthToken, fakeLoginToken)
	cmd.Env = provider.ApplyLoginEnv(cmd.Env, set, unset)
	show("work on a token login", cmd)

	cx := codex.New(userHome)
	cmd, err = cx.Launch(provider.Profile{Name: "codex", Provider: provider.Codex, Home: filepath.Join(userHome, ".codex-team")},
		provider.LaunchRequest{Env: baseEnv, ResumeID: "019a-demo"})
	if err != nil {
		return err
	}
	// Codex manages only CODEX_HOME: Claude's variables pass through untouched.
	show("codex, resuming", cmd)
	return nil
}

// show prints argv, the working dir and the interesting env entries, with
// any credential value replaced by <set>.
func show(title string, cmd provider.Command) {
	argv := append([]string{cmd.Path}, cmd.Args...)
	fmt.Printf("%s:\n  argv: %s\n", title, strings.Join(argv, " "))
	if cmd.Dir != "" {
		fmt.Printf("  dir:  %s\n", cmd.Dir)
	}
	var env []string
	for _, kv := range cmd.Env {
		key, val, _ := strings.Cut(kv, "=")
		for _, v := range shownVars {
			if key != v {
				continue
			}
			if key == claude.OAuthTokenEnv || key == claude.APIKeyEnv {
				val = "<set>"
			}
			env = append(env, key+"="+val)
		}
	}
	sort.Strings(env)
	if len(env) == 0 {
		env = []string{"(none of " + strings.Join(shownVars, ", ") + ")"}
	}
	fmt.Printf("  env:  %s\n", strings.Join(env, " "))
}
