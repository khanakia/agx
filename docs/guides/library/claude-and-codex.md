# Claude Code and Codex providers

Packages `provider/claude` and `provider/codex` are the two built-in implementations of the provider model. They read the logins each vendor CLI stored in its home, fetch plan usage, list resumable conversations, build launch and resume commands, and (for Claude Code) move history when a folder or a conversation moves. You want this page when your own tool needs Claude Code or Codex usage, conversations or launch commands without shelling out to agx.

## Before you start

- Both packages only read logins. They never refresh or write a token, because a refresh rotates the refresh token and logs the vendor CLI out.
- `claude.New` reads the macOS Keychain. Every example below builds the struct itself with `Store: nil`, which disables the Keychain and reads only `<home>/.credentials.json`.
- Programs quoted here: [`examples/usage-fake-server`](../../../examples/usage-fake-server/), [`examples/conversations`](../../../examples/conversations/), [`examples/launch-command`](../../../examples/launch-command/). None of them needs an account or the network.

## Step 1 — Read usage from a home's login

`claude.Provider.Usage` resolves the freshest credential for the profile's home (file, then Keychain when `Store` is set), refuses an expired one locally, and calls the usage endpoint. `UsageClient.Endpoint` is the seam for tests: here it points at an `httptest` server that serves a trimmed copy of a real response.

<!-- snip: examples/usage-fake-server/main.go func:run -->

Source: [`examples/usage-fake-server/main.go`](../../../examples/usage-fake-server/main.go), `run`, quoted verbatim.

```go
func run() error {
	srv := httptest.NewServer(fakeAPI())
	defer srv.Close()

	userHome, err := os.MkdirTemp("", "agx-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	home := filepath.Join(userHome, homeDirName)
	if err := writeCredential(home, fakeHomeToken, time.Now().Add(tokenLifetime)); err != nil {
		return err
	}

	// Store: nil disables the macOS keychain, so only the credentials file
	// in the fake home is read. Endpoint and Probe point at the fake server.
	p := &claude.Provider{
		UserHome:    userHome,
		Store:       nil,
		UsageClient: &claude.UsageClient{Endpoint: srv.URL + usagePath},
		Probe:       &claude.ProbeClient{Endpoint: srv.URL + probePath},
	}
	ctx := context.Background()

	profile := provider.Profile{Name: "work", Provider: provider.Claude, Home: home, Billing: provider.BillingPlan}
	u, err := p.Usage(ctx, profile)
	if err != nil {
		return fmt.Errorf("usage: %w", err)
	}
	fmt.Printf("home login: logged in=%v plan=%q\n", u.Identity.LoggedIn, u.Identity.Plan)
	printWindows(u)

	probed, _, err := p.LoginUsage(ctx, provider.LoginOAuthToken, fakeLoginToken)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	fmt.Println("token login (probe):")
	printWindows(probed)

	limited := &claude.UsageClient{Endpoint: srv.URL + limitedPath}
	_, err = limited.Fetch(ctx, fakeHomeToken)
	fmt.Printf("429: %v (rate-limited=%v)\n", err, errors.Is(err, provider.ErrRateLimited))

	_, err = p.UsageClient.Fetch(ctx, "wrong-token")
	fmt.Printf("bad token: %v (unauthorized=%v)\n", err, errors.Is(err, provider.ErrUnauthorized))
	return nil
}
```

<!-- /snip -->

The fake server and the credential file it reads:

<!-- snip: examples/usage-fake-server/main.go func:fakeAPI -->

Source: [`examples/usage-fake-server/main.go`](../../../examples/usage-fake-server/main.go), `fakeAPI`, quoted verbatim.

```go
// fakeAPI answers the three paths the example calls.
func fakeAPI() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(usagePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeHomeToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(usageBody))
	})
	mux.HandleFunc(probePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeLoginToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		for k, v := range probeHeaders {
			w.Header().Set(k, v)
		}
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc(limitedPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", retryAfterSeconds)
		w.WriteHeader(http.StatusTooManyRequests)
	})
	return mux
}
```

<!-- /snip -->

<!-- snip: examples/usage-fake-server/main.go func:writeCredential -->

Source: [`examples/usage-fake-server/main.go`](../../../examples/usage-fake-server/main.go), `writeCredential`, quoted verbatim.

```go
// writeCredential writes the .credentials.json Claude Code keeps in a home
// when it stores the login on disk.
func writeCredential(home, token string, expires time.Time) error {
	if err := os.MkdirAll(home, homePerm); err != nil {
		return err
	}
	body := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d,"subscriptionType":"max","rateLimitTier":"default_claude_max_20x"}}`,
		token, expires.UnixMilli())
	return os.WriteFile(filepath.Join(home, ".credentials.json"), []byte(body), credentialPerm)
}
```

<!-- /snip -->

Codex works the same way through `codex.Provider{UserHome: ..., Endpoint: ...}`: it reads `<home>/auth.json`, and `Endpoint` overrides `codex.DefaultEndpoint`.

## Step 2 — Read usage for a raw token with the probe

A long-lived token from `claude setup-token` cannot read the usage endpoint, so `LoginUsage` sends one minimal Messages request and reads the `anthropic-ratelimit-unified-*` response headers. `claude.ParseProbeHeaders` maps them onto the same "Current session" and "All models" windows. The example's server sets these headers:

<!-- snip: examples/usage-fake-server/main.go var:probeHeaders -->

Source: [`examples/usage-fake-server/main.go`](../../../examples/usage-fake-server/main.go), `probeHeaders`, quoted verbatim.

```go
// Probe headers: utilization is a 0..1 fraction, reset is unix seconds.
var probeHeaders = map[string]string{
	"anthropic-ratelimit-unified-5h-utilization":       "0.12",
	"anthropic-ratelimit-unified-5h-reset":             "1790242199",
	"anthropic-ratelimit-unified-5h-status":            "allowed",
	"anthropic-ratelimit-unified-7d-utilization":       "0.81",
	"anthropic-ratelimit-unified-7d-reset":             "1790704799",
	"anthropic-ratelimit-unified-7d-status":            "allowed_warning",
	"anthropic-ratelimit-unified-representative-claim": "seven_day",
}
```

<!-- /snip -->

## Step 3 — List conversations

`Conversations` lists the transcripts stored in a home, newest first. With `Dir` set it reads only that folder's history directory, `<home>/projects/<EncodePath(dir)>`, which is how Claude Code itself looks conversations up. Each transcript is read at its head (for the cwd) and its tail (for the title), never whole.

<!-- snip: examples/conversations/main.go var:transcripts -->

Source: [`examples/conversations/main.go`](../../../examples/conversations/main.go), `transcripts`, quoted verbatim.

```go
var transcripts = []transcript{
	{apiDir, "a1", []string{
		`{"type":"user","cwd":"/srv/demo/api","sessionId":"a1"}`,
		`{"type":"ai-title","aiTitle":"Fix login bug"}`,
		// A title the user set with /rename wins over the generated one.
		`{"type":"custom-title","customTitle":"Login fix (final)"}`,
	}, time.Hour},
	{apiDir, "a2", []string{`{"type":"user","cwd":"/srv/demo/api","sessionId":"a2"}`}, 2 * time.Hour},
	{webDir, "w1", []string{
		`{"type":"user","cwd":"/srv/demo/web","sessionId":"w1"}`,
		`{"type":"ai-title","aiTitle":"Add dark mode"}`,
	}, 30 * time.Minute},
	// No record carries a cwd: it cannot be resumed anywhere, so it is skipped.
	{webDir, "w0", []string{`{"type":"summary"}`}, 10 * time.Minute},
}
```

<!-- /snip -->

<!-- snip: examples/conversations/main.go func:writeTranscript -->

Source: [`examples/conversations/main.go`](../../../examples/conversations/main.go), `writeTranscript`, quoted verbatim.

```go
// writeTranscript writes t under home/projects/<EncodePath(dir)>/<id>.jsonl
// and backdates it by t.age.
func writeTranscript(home string, t transcript, now time.Time) error {
	dir := claude.HistoryDir(home, t.dir)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	path := filepath.Join(dir, t.id+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(t.lines, "\n")+"\n"), filePerm); err != nil {
		return err
	}
	mtime := now.Add(-t.age)
	return os.Chtimes(path, mtime, mtime)
}
```

<!-- /snip -->

## Step 4 — Move history

`MoveHistory` re-keys a folder's history after the folder itself moved, with a single rename. `PlanMove` computes a move of a folder's conversations between two homes without changing anything, and `provider.ApplyMovePlan` executes it with an optional backup and rollback. Codex does not implement either: it indexes conversations in SQLite with absolute paths, and `MoveUnsupportedReason` says so.

<!-- snip: examples/conversations/main.go func:run -->

Source: [`examples/conversations/main.go`](../../../examples/conversations/main.go), `run`, quoted verbatim.

```go
func run() error {
	userHome, err := os.MkdirTemp("", "agx-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	tilde := func(p string) string { return strings.Replace(p, userHome, "~", 1) }

	home := claude.DefaultHomeFor(userHome)
	now := time.Now()
	for _, t := range transcripts {
		if err := writeTranscript(home, t, now); err != nil {
			return err
		}
	}
	fmt.Printf("history key for %s: %s\n", apiDir, claude.EncodePath(apiDir))

	p := &claude.Provider{UserHome: userHome} // Store nil: no keychain
	ctx := context.Background()

	all, err := p.Conversations(ctx, home, provider.ConversationQuery{})
	if err != nil {
		return err
	}
	fmt.Println("all conversations, newest first:")
	printConversations(all)

	one, err := p.Conversations(ctx, home, provider.ConversationQuery{Dir: apiDir, Limit: 1})
	if err != nil {
		return err
	}
	fmt.Println("newest in", apiDir+":")
	printConversations(one)

	fmt.Printf("has history: %s=%v /srv/demo/none=%v\n", apiDir, p.HasHistory(home, apiDir), p.HasHistory(home, "/srv/demo/none"))

	// The project folder moved: re-key its history so resume finds it.
	moved, err := p.MoveHistory(home, apiDir, movedDir)
	if err != nil {
		return err
	}
	after, err := p.Conversations(ctx, home, provider.ConversationQuery{Dir: movedDir})
	if err != nil {
		return err
	}
	fmt.Printf("moved=%v; now listed under %s:\n", moved, movedDir)
	printConversations(after)

	// Move web's conversations to a second account's home: plan, check, apply.
	other := filepath.Join(userHome, ".claude-work")
	plan, err := p.PlanMove(ctx, provider.MoveRequest{FromHome: home, ToHome: other, Dir: webDir})
	if err != nil {
		return err
	}
	fmt.Printf("plan: %d conversation(s), %d item(s), %d blocker(s)\n", len(plan.Conversations), len(plan.Items), len(plan.Blockers))
	for _, it := range plan.Items {
		fmt.Printf("  %s\n    → %s\n", tilde(it.From), tilde(it.To))
	}
	backup := filepath.Join(userHome, "move-backup")
	if err := provider.ApplyMovePlan(plan, backup); err != nil {
		return err
	}
	movedConvs, err := p.Conversations(ctx, other, provider.ConversationQuery{Dir: webDir})
	if err != nil {
		return err
	}
	fmt.Printf("after apply, %s holds %d conversation(s) for %s\n", tilde(other), len(movedConvs), webDir)
	return nil
}
```

<!-- /snip -->

## Step 5 — Build launch commands

`Launch` returns a `provider.Command`; it never starts a process. For the default home it removes `CLAUDE_CONFIG_DIR` (or `CODEX_HOME`), for another home it sets it. A token login's variables come from `LoginEnv` and are applied with `provider.ApplyLoginEnv`.

<!-- snip: examples/launch-command/main.go func:run -->

Source: [`examples/launch-command/main.go`](../../../examples/launch-command/main.go), `run`, quoted verbatim.

```go
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
```

<!-- /snip -->

## Try it

```sh
go run ./examples/usage-fake-server
```

<!-- out: usage-fake-server -->

```text
home login: logged in=true plan="Max (20x)"
  session Current session    5.0% normal   resets 2026-09-24T09:29:59Z
  weekly  All models        76.0% warning  resets 2026-09-24T17:59:59Z (binding)
  weekly  Fable             68.0% normal   resets 2026-09-24T17:59:59Z
  max: 76%
token login (probe):
  session Current session   12.0% normal   resets 2026-09-24T09:29:59Z
  weekly  All models        81.0% warning  resets 2026-09-29T17:59:59Z (binding)
  max: 81%
429: usage endpoint rate-limited this account; try again shortly (retry after 30s) (rate-limited=true)
bad token: login rejected by the server (HTTP 401) (unauthorized=true)
```

<!-- /out -->

```sh
go run ./examples/conversations
```

<!-- out: conversations -->

```text
history key for /srv/demo/api: -srv-demo-api
all conversations, newest first:
  w1  /srv/demo/web     "Add dark mode"
  a1  /srv/demo/api     "Login fix (final)"
  a2  /srv/demo/api     ""
newest in /srv/demo/api:
  a1  /srv/demo/api     "Login fix (final)"
has history: /srv/demo/api=true /srv/demo/none=false
moved=true; now listed under /srv/demo/api-v2:
  a1  /srv/demo/api-v2  "Login fix (final)"
  a2  /srv/demo/api-v2  ""
plan: 2 conversation(s), 2 item(s), 0 blocker(s)
  ~/.claude/projects/-srv-demo-web/w0.jsonl
    → ~/.claude-work/projects/-srv-demo-web/w0.jsonl
  ~/.claude/projects/-srv-demo-web/w1.jsonl
    → ~/.claude-work/projects/-srv-demo-web/w1.jsonl
after apply, ~/.claude-work holds 1 conversation(s) for /srv/demo/web
```

<!-- /out -->

```sh
go run ./examples/launch-command
```

<!-- out: launch-command -->

```text
personal (default home):
  argv: claude
  dir:  /srv/demo/api
  env:  ANTHROPIC_API_KEY=<set>
work, resuming a1:
  argv: claude --model opus --resume a1 --verbose
  env:  ANTHROPIC_API_KEY=<set> CLAUDE_CONFIG_DIR=/home/demo/.claude-work
work on a token login:
  argv: claude --model opus --resume a1 --verbose
  env:  CLAUDE_CODE_OAUTH_TOKEN=<set> CLAUDE_CONFIG_DIR=/home/demo/.claude-work
codex, resuming:
  argv: codex resume 019a-demo
  env:  ANTHROPIC_API_KEY=<set> CLAUDE_CONFIG_DIR=/home/demo/.claude-old CODEX_HOME=/home/demo/.codex-team
```

<!-- /out -->

## Pitfalls

- `claude.New(userHome)` sets `Store: claude.Keychain{}`, which runs the macOS `security` tool. On Linux, in CI and in tests, build `&claude.Provider{UserHome: ...}` so `Store` is nil (`provider/claude/claude.go`, `New`).
- Credentials are picked by latest expiry across the file and every Keychain service name, because Claude Code can leave a stale copy in one store; an entry with expiry 0 ranks below any with a known expiry (`provider/claude/credential.go`, `ResolveCredential`, `TestResolveCredential`).
- The Keychain service name for a non-default home hashes the exact, cleaned path Claude Code was given in `CLAUDE_CONFIG_DIR`. A path spelled differently (a symlink, a trailing component) hashes to another name and the login is not found (`provider/claude/credential.go`, `KeychainServices`, `TestKeychainServices`).
- A response with no limit rows is `claude.ErrNoLimits`, not 0% used, so an upstream shape change is never mistaken for an idle account; the probe has the same rule with `claude.ErrProbeHeaders` (`provider/claude/parse.go`, `TestParseUsage_Errors`; `provider/claude/probe.go`, `TestParseProbeHeaders_Matrix`).
- The probe spends a few tokens of the login's plan quota and counts toward rate limits. Call `LoginUsage` only when a user asked for it; an `api-key` login is never probed and returns `provider.ErrAPIBilled` (`provider/claude/probe.go`, `TestLoginUsage_APIKeyNeverProbes`).
- An account at its limit answers the probe with HTTP 429 and the full header set; `Probe` returns those windows (100% used) rather than a rate-limit error (`provider/claude/probe.go`, `TestProbe_429AtLimit`).
- Without `Dir`, a conversation's `Dir` is the cwd recorded in the transcript, which is stale after a folder was promoted or moved; with `Dir`, it is the queried folder (`provider/claude/conversations.go`, `Conversations`, `TestConversations_AfterMove`).
- A transcript with no `cwd` record is not listed, but `PlanMove` still moves it, so a plan can count more conversations than a listing shows (the example plans 2 and lists 1).
- `MoveHistory` never merges: an existing destination is `claude.ErrHistoryExists` (`provider/claude/history.go`, `TestMoveHistoryAndHasHistory`).
- `Launch` handles only the home variable and the profile's own `env`. Credential variables in the base environment (`ANTHROPIC_API_KEY`) pass through until a login's `ApplyLoginEnv` removes them, and Codex passes Claude's variables through untouched (see the launch output above; `provider/claude/claude.go`, `Launch`).
- Codex `HasHistory` builds an in-memory index of every rollout's cwd on first call per home and reuses it for the life of the `Provider` value; create a new value to see rollouts written later (`provider/codex/codex.go`, `HasHistory`).
- The usage endpoints rate-limit when polled repeatedly. Use an `httptest` server while developing (`provider/claude/usage.go`, `TestUsageClientFetch`).

## Related

- [Library overview](README.md)
- [Provider model](provider-model.md)
- [Secrets and logins](secrets-and-logins.md): where a token login's value comes from
- [examples/usage-fake-server](../../../examples/usage-fake-server/), [examples/conversations](../../../examples/conversations/), [examples/launch-command](../../../examples/launch-command/)
