# The provider model

Package `provider` is agx's vendor-neutral vocabulary: a `Provider` is one vendor coding-agent CLI, a `Profile` is one named way to launch it (a home, a billing mode, args, env), and `Usage`, `Window`, `Conversation` and `Command` are the shapes every command works with. You want this page when you add support for another agent CLI, or when you write code that should work for Claude Code and Codex alike without importing either. The walkthrough builds a complete provider for a made-up CLI, `echoagent`, and plugs it into config discovery, auto-pick and launch.

## Before you start

- The package imports only the standard library (enforced by `internal/archtest`), so depending on it costs nothing.
- The program quoted here is [`examples/custom-provider`](../../../examples/custom-provider/). Run it with `go run ./examples/custom-provider`.

## Step 1 — Implement the required interface

`Provider` has seven methods. `Discover` turns what is on disk into profiles, `Identity` reads login state without the network, `Usage` fetches windows, and `Launch` returns a `Command` (data, never a running process).

<!-- snip: provider/provider.go type:Provider -->

Source: [`provider/provider.go`](../../../provider/provider.go), `Provider`, quoted verbatim.

```go
// Provider is implemented once per vendor CLI.
type Provider interface {
	ID() ID
	// Binary is the executable name looked up on PATH.
	Binary() string
	// DefaultHome is the home the vendor CLI uses when no override env var
	// is set.
	DefaultHome(userHome string) string
	// Discover returns the profiles implied by what exists on disk.
	Discover(userHome string) ([]Profile, error)
	// Identity reads login state locally (no network).
	Identity(p Profile) (Identity, error)
	// Usage fetches usage windows. Returns ErrAPIBilled for BillingAPI
	// profiles and ErrNotLoggedIn / ErrExpired when the login is unusable.
	Usage(ctx context.Context, p Profile) (Usage, error)
	// Launch builds the command that starts (or resumes) the vendor CLI.
	Launch(p Profile, req LaunchRequest) (Command, error)
}
```

<!-- /snip -->

The example keeps the vendor's conventions in named constants and declares compile-time checks for every interface it implements, the same guard the built-in providers carry.

<!-- snip: examples/custom-provider/main.go const:echoID -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `echoID`, quoted verbatim.

```go
// echoID names the fake vendor; it is also its config key.
const echoID provider.ID = "echoagent"
```

<!-- /snip -->

<!-- snip: examples/custom-provider/main.go const:echoBinary -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `echoBinary`, quoted verbatim.

```go
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
```

<!-- /snip -->

<!-- snip: examples/custom-provider/main.go type:Echo -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `Echo`, quoted verbatim.

```go
// Echo implements provider.Provider plus the HistoryChecker upgrade.
type Echo struct {
	// UserHome anchors the default home, which Launch must recognise to
	// remove homeEnv rather than set it.
	UserHome string
}
```

<!-- /snip -->

<!-- snip: examples/custom-provider/main.go var:_ -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), the compile-time interface checks, quoted verbatim.

```go
// Compile-time checks, the same guard the built-in providers carry.
var (
	_ provider.Provider       = Echo{}
	_ provider.HistoryChecker = Echo{}
)
```

<!-- /snip -->

<!-- snip: examples/custom-provider/main.go func:Discover -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `Discover`, quoted verbatim.

```go
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
```

<!-- /snip -->

<!-- snip: examples/custom-provider/main.go func:profile -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `profile`, quoted verbatim.

```go
func (Echo) profile(name, home string, isDefault bool) provider.Profile {
	return provider.Profile{
		Name: name, Provider: echoID, Home: home, Billing: provider.BillingPlan,
		Default: isDefault, Source: provider.SourceDiscovered,
	}
}
```

<!-- /snip -->

<!-- snip: examples/custom-provider/main.go func:Identity -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `Identity`, quoted verbatim.

```go
// Identity implements provider.Provider: logged in when usage.json exists.
func (Echo) Identity(p provider.Profile) (provider.Identity, error) {
	_, err := os.Stat(filepath.Join(p.Home, usageFile))
	return provider.Identity{LoggedIn: err == nil}, nil
}
```

<!-- /snip -->

## Step 2 — Report usage with the shared windows and sentinels

A `Window` carries a `Kind` (session, weekly, scoped, other) that policy code reasons about, a `Group` that decides where it renders, a percent and an optional reset. Return the shared sentinel errors (`ErrAPIBilled`, `ErrNotLoggedIn`, `ErrExpired`, `ErrUnauthorized`, `ErrRateLimited`) so pick and the renderers treat your provider like a built-in one.

<!-- snip: examples/custom-provider/main.go func:Usage -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `Usage`, quoted verbatim.

```go
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
```

<!-- /snip -->

`Usage.MaxPercent` is the figure auto-pick ranks by: the busiest window, because any one window at its cap blocks work.

<!-- snip: provider/provider.go func:MaxPercent -->

Source: [`provider/provider.go`](../../../provider/provider.go), `MaxPercent`, quoted verbatim.

```go
// MaxPercent is the highest percent across all windows — the figure that
// decides whether an account can take more work, since any one window at
// 100% blocks it. Zero windows yield 0.
func (u Usage) MaxPercent() float64 {
	highest := 0.0
	for _, w := range u.Windows {
		if w.Percent > highest {
			highest = w.Percent
		}
	}
	return highest
}
```

<!-- /snip -->

## Step 3 — Build the launch command with `HomeEnv`

`HomeEnv` sets the "which home" variable for a non-default home and removes it for the default one. Removing matters: a shell may still export the variable from an earlier session, and for Claude Code even setting `CLAUDE_CONFIG_DIR` to `~/.claude` switches the Keychain entry it reads and makes the account look logged out.

<!-- snip: provider/env.go func:HomeEnv -->

Source: [`provider/env.go`](../../../provider/env.go), `HomeEnv`, quoted verbatim.

```go
// HomeEnv applies a provider's "which config dir" variable (e.g.
// CLAUDE_CONFIG_DIR, CODEX_HOME): set to home when home differs from the
// provider's default, and actively removed otherwise.
//
// Why remove rather than leave alone: a shell may have the variable exported
// from an earlier session, and for Claude Code even setting it to the default
// dir switches the keychain service name and makes the account look logged
// out. The default home must therefore run with the variable absent.
func HomeEnv(env []string, key, home, defaultHome string) []string {
	if filepath.Clean(home) == filepath.Clean(defaultHome) {
		return UnsetEnv(env, key)
	}
	return SetEnv(env, key, home)
}
```

<!-- /snip -->

<!-- snip: examples/custom-provider/main.go func:Launch -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `Launch`, quoted verbatim.

```go
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
```

<!-- /snip -->

## Step 4 — Add optional upgrades only where they are true

Capabilities beyond the core are separate interfaces that agx detects with a type assertion, so a provider that cannot do something is never forced to pretend. The upgrades are `ConversationLister`, `HistoryChecker`, `HistoryMover`, `ConversationMover` (or `MoveUnsupported` to explain why not), `SubscriptionReader`, `LoginLauncher`, `LoginLifetimer` and `LoginUsageReader`. The example implements only `HistoryChecker`:

<!-- snip: examples/custom-provider/main.go func:HasHistory -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `HasHistory`, quoted verbatim.

```go
// HasHistory implements provider.HistoryChecker; this vendor keeps no
// per-folder history, so no session folder is ever held back by it.
func (Echo) HasHistory(string, string) bool { return false }
```

<!-- /snip -->

The two history upgrades, as declared in the model:

<!-- snip: provider/provider.go type:HistoryMover -->

Source: [`provider/provider.go`](../../../provider/provider.go), `HistoryMover`, quoted verbatim.

```go
// HistoryMover is an optional upgrade for providers that key history by
// directory path, so a moved project can take its history along.
type HistoryMover interface {
	// MoveHistory re-keys history recorded for fromDir to toDir. moved is
	// false (with a nil error) when there was no history to move.
	MoveHistory(home, fromDir, toDir string) (moved bool, err error)
}
```

<!-- /snip -->

<!-- snip: provider/provider.go type:HistoryChecker -->

Source: [`provider/provider.go`](../../../provider/provider.go), `HistoryChecker`, quoted verbatim.

```go
// HistoryChecker is an optional upgrade answering "is there any
// conversation started in dir?" cheaply — used by session gc and listing,
// which ask it for every folder and must not read transcripts to do so.
type HistoryChecker interface {
	HasHistory(home, dir string) bool
}
```

<!-- /snip -->

## Step 5 — Plug it into config and pick

`config.Resolve` knows providers only through the interface, so a custom provider's homes are discovered exactly like Claude Code's. The rest of the program fetches usage per profile, lets `pick.Best` choose, and builds the command.

<!-- snip: examples/custom-provider/main.go func:run -->

Source: [`examples/custom-provider/main.go`](../../../examples/custom-provider/main.go), `run`, quoted verbatim.

```go
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
```

<!-- /snip -->

## Try it

```sh
go run ./examples/custom-provider
```

<!-- out: custom-provider -->

```text
echo   default=true  max= 90% err=<nil>
spare  default=false max=  0% err=not logged in
work   default=false max= 35% err=<nil>
auto → work (max 35%) over echo (max 90%)
argv: echoagent --fast
env:  PATH=/usr/bin ECHOAGENT_HOME=~/.echoagent-work
```

<!-- /out -->

## Pitfalls

- An empty `Usage` with a nil error scores 0% and wins auto-pick. A provider must return an error when the vendor's response has no windows, never an empty snapshot: Claude's parser returns `claude.ErrNoLimits` and Codex's `codex.ErrNoWindows` for exactly this reason (`provider/claude/parse.go`, `ParseUsage`; `provider/provider.go`, `MaxPercent`, pinned by `TestMaxPercent`).
- `ErrRateLimited` is transient (HTTP 429 from polling) and never a login problem; callers must not suggest re-logging in for it. Build it with `provider.RateLimitError(resp.Header)` so the `Retry-After` hint is kept (`provider/ratelimit.go`, `TestRateLimitError`).
- Set the home variable for the default home and Claude Code reads a different Keychain entry; leave it alone and a stale value from the shell wins. Use `HomeEnv`, which removes it (`provider/env.go`, `TestHomeEnv`).
- `Command.Path` is a bare binary name. Resolve it with `exec.LookPath` before `syscall.Exec`; `Launch` does not (`provider/provider.go`, `Command`).
- `SetEnv`, `UnsetEnv` and `ApplyLoginEnv` return copies and never modify the input slice, so a base environment can be reused across launches (`provider/env.go`, `TestSetAndUnsetEnv`; `provider/login.go`, `TestApplyLoginEnv_Matrix`).
- `ApplyMovePlan` refuses a plan with blockers before touching a file, and rolls back already-renamed items when one rename fails (`provider/move.go`, `TestApplyMovePlan_RefusesBlockedPlan`, `TestApplyMovePlan_RollsBackOnFailure`).
- Keep `provider` free of third-party imports if you fork it: `TestLayering` fails the build otherwise (`internal/archtest/archtest_test.go`).

## Related

- [Library overview](README.md)
- [Claude Code and Codex](claude-and-codex.md): the two built-in implementations
- [Config, pick and sessions](config-pick-sessions.md)
- [examples/custom-provider](../../../examples/custom-provider/)
