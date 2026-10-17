// Command custom-provider implements provider.Provider for a made-up vendor
// CLI ("echoagent") and plugs it into the same machinery agx uses for
// Claude Code and Codex: config.Resolve discovers its homes, pick.Best
// chooses the account with the most headroom, and Launch builds the command.
//
// The vendor keeps one home per account (~/.echoagent, ~/.echoagent-<name>)
// with a usage.json file standing in for its usage API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/pick"
	"github.com/khanakia/agx/provider"
)

// echoID names the fake vendor; it is also its config key.
const echoID provider.ID = "echoagent"

// The fake vendor's other conventions, in one place.
const (
	echoBinary  = "echoagent"
	defaultDir  = ".echoagent"
	extraPrefix = ".echoagent-"
	// homeEnv selects a non-default home, like CLAUDE_CONFIG_DIR.
	homeEnv = "ECHOAGENT_HOME"
	// usageFile is what Usage reads; a real provider calls its API here.
	usageFile = "usage.json"
	// defaultName is the profile name of the default home.
	defaultName = "echo"
)

// File modes for the fake homes.
const (
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600
)

// Echo implements provider.Provider plus the HistoryChecker upgrade.
type Echo struct {
	// UserHome anchors the default home, which Launch must recognise to
	// remove homeEnv rather than set it.
	UserHome string
}

// Compile-time checks, the same guard the built-in providers carry.
var (
	_ provider.Provider       = Echo{}
	_ provider.HistoryChecker = Echo{}
)

// ID implements provider.Provider.
func (Echo) ID() provider.ID { return echoID }

// Binary implements provider.Provider.
func (Echo) Binary() string { return echoBinary }

// DefaultHome implements provider.Provider.
func (Echo) DefaultHome(userHome string) string { return filepath.Join(userHome, defaultDir) }

// Discover implements provider.Provider: every home on disk is a profile.
func (e Echo) Discover(userHome string) ([]provider.Profile, error) {
	dirs, err := filepath.Glob(filepath.Join(userHome, extraPrefix+"*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)
	var out []provider.Profile
	if info, err := os.Stat(e.DefaultHome(userHome)); err == nil && info.IsDir() {
		out = append(out, e.profile(defaultName, e.DefaultHome(userHome), true))
	}
	for _, d := range dirs {
		out = append(out, e.profile(strings.TrimPrefix(filepath.Base(d), extraPrefix), d, false))
	}
	return out, nil
}

func (Echo) profile(name, home string, isDefault bool) provider.Profile {
	return provider.Profile{
		Name: name, Provider: echoID, Home: home, Billing: provider.BillingPlan,
		Default: isDefault, Source: provider.SourceDiscovered,
	}
}

// Identity implements provider.Provider: logged in when usage.json exists.
func (Echo) Identity(p provider.Profile) (provider.Identity, error) {
	_, err := os.Stat(filepath.Join(p.Home, usageFile))
	return provider.Identity{LoggedIn: err == nil}, nil
}

// wireUsage is the fake vendor's usage payload.
type wireUsage struct {
	Daily  float64 `json:"daily"`
	Weekly float64 `json:"weekly"`
}

// Usage implements provider.Provider with the shared sentinels, so agx's
// renderers and pick treat it like any built-in provider.
func (Echo) Usage(_ context.Context, p provider.Profile) (provider.Usage, error) {
	if p.Billing == provider.BillingAPI {
		return provider.Usage{}, provider.ErrAPIBilled
	}
	raw, err := os.ReadFile(filepath.Join(p.Home, usageFile))
	if errors.Is(err, os.ErrNotExist) {
		return provider.Usage{}, provider.ErrNotLoggedIn
	}
	if err != nil {
		return provider.Usage{}, err
	}
	var w wireUsage
	if err := json.Unmarshal(raw, &w); err != nil {
		return provider.Usage{}, fmt.Errorf("echoagent: decode usage: %w", err)
	}
	return provider.Usage{
		Identity: provider.Identity{LoggedIn: true},
		Windows: []provider.Window{
			{Kind: provider.WindowSession, Group: provider.GroupSession, Label: "Today", Percent: w.Daily},
			{Kind: provider.WindowWeekly, Group: provider.GroupWeekly, Label: "This week", Percent: w.Weekly},
		},
	}, nil
}

// Launch implements provider.Provider: data only, never executed here.
func (e Echo) Launch(p provider.Profile, req provider.LaunchRequest) (provider.Command, error) {
	args := append([]string{}, p.Args...)
	if req.ResumeID != "" {
		args = append(args, "--resume", req.ResumeID)
	}
	args = append(args, req.Args...)
	env := provider.HomeEnv(req.Env, homeEnv, p.Home, e.DefaultHome(e.UserHome))
	for k, v := range p.Env {
		env = provider.SetEnv(env, k, v)
	}
	return provider.Command{Path: echoBinary, Args: args, Env: env, Dir: req.Dir}, nil
}

// HasHistory implements provider.HistoryChecker; this vendor keeps no
// per-folder history, so no session folder is ever held back by it.
func (Echo) HasHistory(string, string) bool { return false }

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
	homes := map[string]string{
		defaultDir:            `{"daily": 10, "weekly": 90}`,
		extraPrefix + "work":  `{"daily": 35, "weekly": 20}`,
		extraPrefix + "spare": "", // no usage.json: not logged in
	}
	for dir, body := range homes {
		if err := os.MkdirAll(filepath.Join(userHome, dir), dirPerm); err != nil {
			return err
		}
		if body == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(userHome, dir, usageFile), []byte(body), filePerm); err != nil {
			return err
		}
	}

	// No config file: discovery alone produces the profiles.
	echo := Echo{UserHome: userHome}
	cfg, err := config.Resolve(config.File{}, userHome, []provider.Provider{echo})
	if err != nil {
		return err
	}
	ctx := context.Background()
	var cands []pick.Candidate
	for _, p := range cfg.Profiles {
		u, err := echo.Usage(ctx, p)
		cands = append(cands, pick.Candidate{Profile: p, Usage: u, Err: err})
		fmt.Printf("%-6s default=%-5v max=%3.0f%% err=%v\n", p.Name, p.Default, u.MaxPercent(), err)
	}
	res, err := pick.Best(cands)
	if err != nil {
		return err
	}
	fmt.Println(res.Explain())

	cmd, err := echo.Launch(res.Chosen, provider.LaunchRequest{Env: []string{"PATH=/usr/bin"}, Args: []string{"--fast"}})
	if err != nil {
		return err
	}
	fmt.Printf("argv: %s %s\nenv:  %s\n", cmd.Path, strings.Join(cmd.Args, " "), strings.Replace(strings.Join(cmd.Env, " "), userHome, "~", 1))
	return nil
}
