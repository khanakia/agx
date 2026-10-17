// Command config-resolve shows how package config turns an optional YAML
// file plus what each provider discovers on disk into the final profile
// list: with no file every discovered home is a profile; once a provider has
// a configured profile its discovered ones are dropped; unknown keys and
// unsafe alias values are refused.
//
// Everything runs against a throwaway home directory; the keychain is
// disabled and nothing is launched or fetched.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
	"github.com/khanakia/agx/provider/codex"
)

// File modes for the fake home: private dirs, ordinary files.
const (
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600
)

// fakeHome lists the files that make each directory look like a real
// vendor home. ~/.claude-worktrees has no config marker, so discovery
// skips it the way it skips git worktrees on a real machine.
var fakeHome = map[string]string{
	".claude/settings.json":      "{}",
	".claude-work/settings.json": "{}",
	".claude-worktrees/README":   "not an account",
	".codex/auth.json":           "{}",
}

// configYAML configures two Claude profiles on one home (a plan login and
// an api-billed backend) plus an alias. Codex has no configured profile, so
// its discovered one stays.
const configYAML = `sessions:
  root: ~/work/sessions
profiles:
  - name: main
    provider: claude
    home: ~/.claude-work
  - name: kimi
    provider: claude
    home: ~/.claude-work
    billing: api
    env:
      ANTHROPIC_BASE_URL: https://api.example.test/anthropic
    secrets:
      ANTHROPIC_AUTH_TOKEN: env:KIMI_TOKEN
shell:
  aliases:
    clm: run -p main
`

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	userHome, err := os.MkdirTemp("", "agx-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	if err := writeTree(userHome, fakeHome); err != nil {
		return err
	}
	tilde := func(p string) string { return strings.Replace(p, userHome, "~", 1) }

	// Store: nil keeps discovery and identity off the macOS keychain.
	providers := []provider.Provider{&claude.Provider{UserHome: userHome}, codex.New(userHome)}
	agxDir := filepath.Join(userHome, ".agx")
	path := filepath.Join(agxDir, config.FileName)

	// 1. No config file: a missing file is not an error, and every
	// discovered home becomes a profile.
	cfg, err := config.Load(path, userHome, providers)
	if err != nil {
		return err
	}
	fmt.Printf("no file (found=%v), sessions root %s\n", cfg.Found, tilde(cfg.SessionsRoot))
	printProfiles(cfg, tilde)

	// 2. With a config file: Claude's discovered homes are no longer added.
	if err := writeTree(agxDir, map[string]string{config.FileName: configYAML}); err != nil {
		return err
	}
	cfg, err = config.Load(path, userHome, providers)
	if err != nil {
		return err
	}
	fmt.Printf("with file (found=%v), sessions root %s, promote root %s\n", cfg.Found, tilde(cfg.SessionsRoot), tilde(cfg.PromoteRoot))
	printProfiles(cfg, tilde)
	if p, ok := cfg.ForHome(provider.Claude, filepath.Join(userHome, ".claude-work")); ok {
		fmt.Printf("resume in ~/.claude-work runs on: %s (plan before api)\n", p.Name)
	}
	for _, a := range cfg.Aliases {
		fmt.Printf("alias %s → agx %s\n", a.Name, a.Args)
	}

	// 3. Strict decoding: a misspelt key is an error, never ignored.
	_, err = config.Decode([]byte("profile:\n  - name: main\n"))
	fmt.Printf("typo: %v (invalid=%v)\n", err, errors.Is(err, config.ErrInvalid))

	// 4. Alias values are pasted into shell code, so $, quotes, ; and | are refused.
	f, err := config.Decode([]byte("shell:\n  aliases:\n    bad: run -p main; rm -rf ~\n"))
	if err != nil {
		return err
	}
	_, err = config.Resolve(f, userHome, providers)
	fmt.Printf("unsafe alias: %v\n", err)
	return nil
}

// printProfiles prints one line per resolved profile.
func printProfiles(cfg config.Config, tilde func(string) string) {
	for _, p := range cfg.Profiles {
		fmt.Printf("  %-8s %-6s %-15s %-4s default=%-5v %s\n", p.Name, p.Provider, tilde(p.Home), p.Billing, p.Default, p.Source)
	}
}

// writeTree creates each relative path under root with its content.
func writeTree(root string, files map[string]string) error {
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), filePerm); err != nil {
			return err
		}
	}
	return nil
}
