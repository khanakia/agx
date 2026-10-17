# Library or CLI

Everything agx does is available two ways: run the `agx` binary (and read its `--json` output), or import its Go packages and call them yourself. The CLI gives you agx's finished decisions (login precedence, health checks, the picker, the shell layer); the library gives you the building blocks, with fakes and injection points, but leaves the wiring to you. This page helps you choose, and names the packages and the JSON kinds for each job. Read it before writing a status-bar script, a dashboard, a wrapper around Claude Code or Codex, or support for another agent CLI.

## Before you start

- For the CLI route: agx on `PATH` and `jq` (or any JSON parser); see [JSON output and scripting](../guides/usage/json-and-scripting.md).
- For the library route: Go, and `go get github.com/khanakia/agx@<version>`; see [Use agx as a Go library](../guides/library/README.md).
- The module is at a `v0` version: pin the version you build against in `go.mod`, and read the [CHANGELOG](../../CHANGELOG.md) before upgrading.

## Step 1 — Decide by what you are building

| You are building | Use | Why | Guide | Example |
|---|---|---|---|---|
| A shell script, tmux or status-bar line, cron job, or a tool in another language | the CLI with `--json` | one process call; the envelope carries `schema_version`, and unknown keys can be ignored | [JSON output and scripting](../guides/usage/json-and-scripting.md#step-3--use-the-jq-recipes) | — |
| Something that must behave exactly like `agx run` / `resume` (logins, defaults, `AGX_LOGIN`, the shell layer) | the CLI | login precedence, the doctor checks and the launch-plan protocol live in `internal/cli` and `internal/shellinit`, which Go cannot import | [Choosing a login](../guides/logins/choosing-a-login.md) | — |
| A Go program that reads usage, conversations or config | the library | typed results, no subprocess, no text parsing | [Claude Code and Codex](../guides/library/claude-and-codex.md) | [usage-fake-server](../../examples/usage-fake-server/), [conversations](../../examples/conversations/), [config-resolve](../../examples/config-resolve/) |
| Your own auto-pick across accounts, with your own candidates | the library: `pick.Best` | pure function, no I/O; feed it any usage | [Config, pick and sessions](../guides/library/config-pick-sessions.md#step-2--choose-a-profile-with-pickbest) | [autopick](../../examples/autopick/) |
| A launcher that builds the agent's command line and env | the library: `Launch` + `provider.HomeEnv` | returns a `provider.Command` (argv, dir, env) for you to exec | [Claude Code and Codex: launch commands](../guides/library/claude-and-codex.md#step-5--build-launch-commands) | [launch-command](../../examples/launch-command/) |
| Support for another agent CLI | the library: implement `provider.Provider` | config resolution and `pick` then work with it unchanged | [The provider model](../guides/library/provider-model.md) | [custom-provider](../../examples/custom-provider/) |
| Session-folder housekeeping in your own tool | the library: `sessions` | `Create`, `List`, `PlanGC`, `RemoveEmpty`, `Promote`; history is asked through a callback | [Config, pick and sessions](../guides/library/config-pick-sessions.md#step-3--create-list-and-collect-session-folders) | [sessions-gc](../../examples/sessions-gc/) |
| Token handling for named logins | the library: `login` | `Resolver`, `TokenFile` (0600, permission-checked), `Compatible` | [Secrets and logins](../guides/library/secrets-and-logins.md) | [login-resolve](../../examples/login-resolve/) |

If X then Y:

- If you only read data, start with the CLI: `agx usage --json`, `agx profiles --json` and `agx resume --list --json` cover most dashboards, and you get agx's fixes for free when you upgrade the binary.
- If you need to change how a decision is made (score, eligibility, which homes count), use the library and own that code.
- If you need both, mix them: call the library for data and `agx run --dry-run` (or `agx` itself) for launches, so the launch rules stay agx's.

## Step 2 — Know what only one side has

| Only in the CLI | Only in the library |
|---|---|
| Login precedence (`--login`, `AGX_LOGIN`, profile, provider, home) and `--login auto` | Injection points: `claude.UsageClient.Endpoint`, `ProbeClient.Endpoint`, `Provider.Store = nil` (no Keychain), `Provider.Now`, `secret.System.LookupEnv`, `login.Resolver.LookupEnv` / `RunCmd` |
| `agx doctor` checks and their levels | Your own `provider.Provider` implementations |
| The fzf picker and its numbered fallback | `provider.MovePlan` to inspect a move before `ApplyMovePlan` runs it |
| The shell layer: landing in the folder, OSC 7, aliases, `aglogin`, completion | `config.Editor` for comment-preserving edits of your own |
| `exec` replacement of the agx process by the agent | A `provider.Command` you can run, log or wrap |
| Concurrent usage fetch for every profile with one timeout each | The plain `Usage` call per profile; the concurrency is yours |

The split is enforced: a test walks every package and fails if a library imports `internal/`, cobra or voltkit (`internal/archtest/archtest_test.go`, `TestLayering`; quoted in [Respect the layering](../guides/library/README.md#step-2--respect-the-layering)).

## Step 3 — Map the CLI output to the library call

| CLI | JSON kind | Library equivalent |
|---|---|---|
| `agx usage --json` | `usage.list` | `Provider.Usage(ctx, profile)` per profile |
| `agx profiles --json` | `profile.list` | `config.Load` (discovery + config) and `Provider.Identity` |
| `agx resume --list --json` | `conversation.list` | `claude.Provider.Conversations` (a `provider.ConversationLister`) |
| `agx sessions ls --json` / `gc --json` | `session.list` / `session.gc` | `sessions.List` / `sessions.PlanGC` |
| `agx sessions move --dry-run --json` | `session.move` | `PlanMove` + `provider.ApplyMovePlan` |
| `agx login ls --json` | `login.list` | `config.Load` (`Logins`) + `login.ExpiresAt` |
| `agx run -p auto --dry-run` | — (text) | `pick.Best` + `Result.Explain`, then `Launch` |

Field-level shapes of every kind are in [Read the kind you need](../guides/usage/json-and-scripting.md#step-2--read-the-kind-you-need).

## Try it

The CLI route, on a throwaway home (the home folder is shown as `/home/you`), first entries of `agx profiles --json`:

```text
$ agx profiles --json | head -20
{
  "schema_version": 1,
  "kind": "profile.list",
  "count": 3,
  "data": [
    {
      "name": "personal",
      "provider": "claude",
      "home": "/home/you/.claude",
      "billing": "plan",
      "default": true,
      "source": "config",
      "login": "none",
      "login_name": "home",
      "login_source": "home"
    },
    {
      "name": "work",
      "provider": "claude",
      "home": "/home/you/.claude-work",
```

The library route: `go run ./examples/autopick` from the repository root feeds `pick.Best` fixed usage and prints its decisions:

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

## Pitfalls

- **`claude.New` reads the macOS Keychain.** In tests and services build `&claude.Provider{UserHome: …}` with `Store` left nil, as every example does ([Use agx as a Go library: pitfalls](../guides/library/README.md#pitfalls)).
- **The library talks to the real usage endpoints unless you point it elsewhere.** They rate-limit when polled (`provider.ErrRateLimited`); use an `httptest` server while developing ([usage-fake-server](../../examples/usage-fake-server/)).
- **A wrapper must remove, not set, the home variable for the default home.** Setting `CLAUDE_CONFIG_DIR=~/.claude` makes the account look logged out; `provider.HomeEnv` does the right thing ([The provider model](../guides/library/provider-model.md#step-3--build-the-launch-command-with-homeenv)).
- **Scripts should test exit codes and tolerate new keys.** `0` ok, `1` a runtime failure (some account errored, nothing eligible), `2` a usage or config error ([Branch on the exit status](../guides/usage/json-and-scripting.md#step-4--branch-on-the-exit-status)); optional fields are left out when empty.
- **Probing a token login spends quota** from the CLI (`agx usage --logins`) and the library (`LoginUsage`) alike: one minimal request per call.

## Related

- [Use agx as a Go library](../guides/library/README.md)
- [JSON output and scripting](../guides/usage/json-and-scripting.md)
- [Examples](../../examples/README.md)
