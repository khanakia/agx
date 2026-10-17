# Config, auto-pick and session folders

Three small packages carry agx's decisions. `config` merges the optional `config.yaml` with what each provider discovers on disk into the final profile list. `pick` chooses the profile with the most headroom from their usage. `sessions` creates, lists, garbage-collects and promotes the timestamped folders `agx new` works in. You want this page when your tool should see the same profiles agx sees, choose an account the same way, or manage session folders without the CLI.

## Before you start

- `config` knows providers only through `provider.Provider`; pass the ones you use (the built-ins, your own, or both).
- `pick` is pure (no I/O). `sessions` touches only the filesystem and asks about history through a callback, so neither imports a provider.
- Programs quoted here: [`examples/config-resolve`](../../../examples/config-resolve/), [`examples/autopick`](../../../examples/autopick/), [`examples/sessions-gc`](../../../examples/sessions-gc/).

## Step 1 — Load and resolve the config

`config.Load` reads a file (a missing one is "no config", reported by `Found`), decodes it strictly and calls `config.Resolve`. With no file every discovered home becomes a profile; once a provider has one configured profile, its discovered ones are no longer added. The example's config file:

<!-- snip: examples/config-resolve/main.go const:configYAML -->

Source: [`examples/config-resolve/main.go`](../../../examples/config-resolve/main.go), `configYAML`, quoted verbatim.

```go
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
```

<!-- /snip -->

<!-- snip: examples/config-resolve/main.go func:run -->

Source: [`examples/config-resolve/main.go`](../../../examples/config-resolve/main.go), `run`, quoted verbatim.

```go
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
```

<!-- /snip -->

For in-place edits that keep the user's comments and key order, `config.Editor{Path, UserHome, Providers}` offers `AddLogin`, `RemoveLogin`, `SetProfileLogin` and `SetProviderLogin`; every edit is validated by a full decode and resolve before it is written.

## Step 2 — Choose a profile with `pick.Best`

`pick.Best` takes candidates in config order, each with its usage or its fetch error. The score is `Usage.MaxPercent()`, the lowest score wins, ties keep candidate order, and `api`-billed or failed candidates are skipped with a reason. `Result.Explain` renders the one line agx prints for `run -p auto`.

<!-- snip: examples/autopick/main.go func:run -->

Source: [`examples/autopick/main.go`](../../../examples/autopick/main.go), `run`, quoted verbatim.

```go
func run() error {
	cands := []pick.Candidate{
		// personal: the 5-hour window is nearly free, but the weekly one is
		// at 77%, and any window at its cap blocks work — so it scores 77.
		{Profile: profile("personal", provider.BillingPlan), Usage: usage(4, 77)},
		{Profile: profile("work", provider.BillingPlan), Usage: usage(5, 12)},
		// kimi is billed per token: never chosen, whatever its usage says.
		{Profile: profile("kimi", provider.BillingAPI)},
		// A failed fetch makes a candidate ineligible; the error is kept.
		{Profile: profile("spare", provider.BillingPlan), Err: provider.ErrNotLoggedIn},
	}

	res, err := pick.Best(cands)
	if err != nil {
		return err
	}
	fmt.Println(res.Explain())
	fmt.Printf("chosen=%s score=%.0f\n", res.Chosen.Name, res.Score)
	for _, c := range cands {
		if reason, ok := res.Skipped[c.Profile.Name]; ok {
			fmt.Printf("skipped %s: %v\n", c.Profile.Name, reason)
		}
	}

	// Ties keep candidate (config) order: "a" and "b" both score 40.
	tie, err := pick.Best([]pick.Candidate{
		{Profile: profile("a", provider.BillingPlan), Usage: usage(40, 10)},
		{Profile: profile("b", provider.BillingPlan), Usage: usage(10, 40)},
	})
	if err != nil {
		return err
	}
	fmt.Println(tie.Explain())

	// Nobody eligible: ErrNoCandidate, with every reason in the message.
	_, err = pick.Best(cands[2:])
	fmt.Printf("none eligible: %v (no-candidate=%v)\n", err, errors.Is(err, pick.ErrNoCandidate))
	return nil
}
```

<!-- /snip -->

<!-- snip: examples/autopick/main.go func:usage -->

Source: [`examples/autopick/main.go`](../../../examples/autopick/main.go), `usage`, quoted verbatim.

```go
// usage builds a snapshot with a session and a weekly window.
func usage(session, weekly float64) provider.Usage {
	return provider.Usage{Windows: []provider.Window{
		{Kind: provider.WindowSession, Group: provider.GroupSession, Label: "Current session", Percent: session},
		{Kind: provider.WindowWeekly, Group: provider.GroupWeekly, Label: "All models", Percent: weekly},
	}}
}
```

<!-- /snip -->

## Step 3 — Create, list and collect session folders

`sessions.Create` names a folder `[slug_]YYYYMMDD_HHMMSS` and adds a numeric suffix when the name is taken. `List` reports files and, through your `HistoryFunc`, whether any provider has history there. `PlanGC` decides without changing anything; `RemoveEmpty` deletes only what is still empty; `Promote` moves a folder with one rename.

<!-- snip: examples/sessions-gc/main.go func:run -->

Source: [`examples/sessions-gc/main.go`](../../../examples/sessions-gc/main.go), `run`, quoted verbatim.

```go
func run() error {
	tmp, err := os.MkdirTemp("", "agx-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	root := filepath.Join(tmp, "sessions")

	// Creation time is part of the name, so it is fixed here; gc reads it
	// back from the name, not from the folder's mtime.
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local)
	now := base.Add(5 * day)

	if _, err := sessions.Create(root, sessions.Slug("Try", "the new API!"), base); err != nil {
		return err
	}
	withFile, err := sessions.Create(root, "notes", base.Add(day))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(withFile, "plan.md"), []byte("# plan\n"), filePerm); err != nil {
		return err
	}
	withHistory, err := sessions.Create(root, "chat", base.Add(2*day))
	if err != nil {
		return err
	}
	if _, err := sessions.Create(root, "", now.Add(-time.Hour)); err != nil { // too new
		return err
	}
	// Two creates in the same second get a numeric suffix, never a shared folder.
	twin, err := sessions.Create(root, "chat", base.Add(2*day))
	if err != nil {
		return err
	}
	fmt.Printf("same-second twin: %s\n", filepath.Base(twin))

	hasHistory := func(dir string) bool { return dir == withHistory }
	folders, err := sessions.List(root, hasHistory)
	if err != nil {
		return err
	}
	plan := sessions.PlanGC(folders, now, minAge)
	for _, it := range plan {
		fmt.Printf("  %-32s files=%d remove=%-5v %s\n", it.Folder.Name, it.Folder.Files, it.Remove, it.Reason)
	}
	for _, it := range plan {
		if !it.Remove {
			continue
		}
		if err := sessions.RemoveEmpty(it.Folder.Path); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", it.Folder.Name)
	}

	// RemoveEmpty re-checks at delete time: a folder that gained a file
	// after planning is refused, not deleted.
	err = sessions.RemoveEmpty(withFile)
	fmt.Printf("remove non-empty: refused=%v\n", err != nil)

	// Promote is one rename on the same disk and never overwrites.
	dst := filepath.Join(tmp, "projects", "notes-app")
	if err := sessions.Promote(withFile, dst); err != nil {
		return err
	}
	fmt.Printf("promoted %s → projects/%s\n", filepath.Base(withFile), filepath.Base(dst))
	again, err := sessions.Create(root, "notes", now)
	if err != nil {
		return err
	}
	err = sessions.Promote(again, dst)
	fmt.Printf("promote onto existing: exists=%v\n", errors.Is(err, sessions.ErrExists))
	return nil
}
```

<!-- /snip -->

In agx the `HistoryFunc` asks every provider that implements `provider.HistoryChecker` for every home it knows.

## Try it

```sh
go run ./examples/config-resolve
```

<!-- out: config-resolve -->

```text
no file (found=false), sessions root ~/agx-sessions
  personal claude ~/.claude       plan default=true  discovered
  work     claude ~/.claude-work  plan default=false discovered
  codex    codex  ~/.codex        plan default=true  discovered
with file (found=true), sessions root ~/work/sessions, promote root ~/work
  main     claude ~/.claude-work  plan default=true  config
  kimi     claude ~/.claude-work  api  default=false config
  codex    codex  ~/.codex        plan default=true  discovered
resume in ~/.claude-work runs on: main (plan before api)
alias clm → agx run -p main
typo: config: invalid: yaml: unmarshal errors:
  line 1: field profile not found in type config.File (invalid=true)
unsafe alias: config: invalid: shell.aliases.bad: only letters, digits, spaces and _./:=@+- are allowed
```

<!-- /out -->

```sh
go run ./examples/autopick
```

<!-- out: autopick -->

```text
auto → work (max 12%) over personal (max 77%)
chosen=work score=12
skipped kimi: billed per token: no usage windows
skipped spare: not logged in
auto → a (max 40%) over b (max 40%)
none eligible: pick: no eligible profile: kimi: billed per token: no usage windows; spare: not logged in (no-candidate=true)
```

<!-- /out -->

```sh
go run ./examples/sessions-gc
```

<!-- out: sessions-gc -->

```text
same-second twin: chat_20261003_090000_2
  20261006_080000                  files=0 remove=false newer than min age
  chat_20261003_090000             files=0 remove=false has conversation history
  chat_20261003_090000_2           files=0 remove=true  empty, no history
  notes_20261002_090000            files=1 remove=false has files
  try_the_new_api_20261001_090000  files=0 remove=true  empty, no history
removed chat_20261003_090000_2
removed try_the_new_api_20261001_090000
remove non-empty: refused=true
promoted notes_20261002_090000 → projects/notes-app
promote onto existing: exists=true
```

<!-- /out -->

## Pitfalls

- One configured profile for a provider switches off discovery for that provider entirely: the config becomes the complete list (`config/config.go`, `Resolve`, `TestResolve_FullConfig`).
- A discovered profile whose name collides with a configured one is renamed `<provider>-<name>` (for example `codex-personal`), not dropped (`config/config.go`, `Resolve`, `TestResolve_DiscoveredNameCollision`).
- Unknown keys are errors wrapping `config.ErrInvalid`, so a misspelt key never silently does nothing (`config/config.go`, `Decode`, `TestResolve_Invalid`).
- Two profiles marked `default: true` for one provider is an error; with none, the first profile of that provider becomes the default (`config/config.go`, `settleDefaults`, `TestResolve_FirstProfileBecomesDefault`).
- Alias values are pasted into generated shell code, so only letters, digits, spaces and `_./:=@+-` are accepted (`config/config.go`, `aliasArgsPattern`).
- `auto` is reserved as a profile name and `home` / `auto` as login names (`config/config.go`, `resolveProfile`; `config/logins.go`, `resolveLogins`).
- `pick.Best` never chooses an `api`-billed profile, whatever its usage says (`pick/pick.go`, `TestBest_APIBilledNeverChosen`).
- Passing a nil `HistoryFunc` to `sessions.List` reports every folder as having no history, so `PlanGC` will mark an empty folder that holds conversations as collectable. Always pass a real history check before collecting (`sessions/sessions.go`, `List`).
- Folder age comes from the timestamp in the name, parsed in local time, and falls back to mtime only for hand-made names (`sessions/sessions.go`, `sortKey`, `TestListAndPlanGC`).
- `RemoveEmpty` re-checks at delete time and refuses a folder that gained a file after planning; `.DS_Store` does not count as content (`sessions/sessions.go`, `TestRemoveEmpty`).
- `Promote` refuses an existing destination (`sessions.ErrExists`) and a destination on another disk (`sessions.ErrCrossDevice`): it is one atomic rename and never copies (`sessions/sessions.go`, `TestPromote`; `sessions/crossdevice_unix_test.go`, `TestRenameError_CrossDevice`).

## Related

- [Library overview](README.md)
- [Provider model](provider-model.md)
- [examples/config-resolve](../../../examples/config-resolve/), [examples/autopick](../../../examples/autopick/), [examples/sessions-gc](../../../examples/sessions-gc/)
