<h1 align="center">claude-usage</h1>
<p align="center"><strong>Every Claude account's plan limits, in one terminal command.</strong></p>
<p align="center">A CLI that shows your Claude Pro / Max plan usage limits (current 5-hour session, weekly across all models, and weekly per model) for every Claude Code account on your machine at once.</p>

<p align="center">
  <a href="https://github.com/khanakia/claude-usage/releases"><img src="https://img.shields.io/github/v/release/khanakia/claude-usage?include_prereleases&color=2563eb" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-2563eb" alt="License: Apache-2.0"></a>
  <img src="https://img.shields.io/badge/go-%E2%89%A51.25-2563eb?logo=go&logoColor=white" alt="Go 1.25 or newer">
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-2563eb" alt="Platforms: macOS and Linux">
  <img src="https://img.shields.io/badge/dependencies-zero-2563eb" alt="Zero dependencies">
  <a href="https://github.com/khanakia/claude-usage/stargazers"><img src="https://img.shields.io/github/stars/khanakia/claude-usage?style=flat&color=2563eb" alt="GitHub stars"></a>
</p>

**claude-usage** is an open-source Claude usage tracker for the terminal. It shows the same numbers as the claude.ai **Settings → Usage** page and Claude Code's `/usage` command (percent of each plan window used, and when it resets), but for **all** of your Claude Code accounts side by side. If you keep separate personal and work logins with `CLAUDE_CONFIG_DIR` (`~/.claude`, `~/.claude-work`, …), one command tells you which account still has headroom. It is a single Go binary with no dependencies, it only reads the login Claude Code already stored, and it has a `--json` mode for scripts, prompts, and status bars.

```text
$ claude-usage
you@example.com  Max (20x)  ~/.claude
  Current session    ███░░░░░░░░░░░░░░░░░░░░░░░░░░░   10% used  resets in 4 hr 7 min
  Weekly limits
  All models         ███████████████████████░░░░░░░   77% used  resets in 12 hr 37 min
  Fable              █████████████████████░░░░░░░░░   69% used  resets in 12 hr 37 min

you@work.example  Max (20x)  ~/.claude-work
  Current session    ██░░░░░░░░░░░░░░░░░░░░░░░░░░░░    5% used  resets in 4 hr 7 min
  Weekly limits
  All models         █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    2% used  resets in 6 d 0 hr
  Fable              ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    0% used  resets in 6 d 0 hr
```

In a terminal the bars are coloured by the server's own severity: blue when normal, amber on a warning, red beyond that.

## Contents

- [Why claude-usage?](#why-claude-usage)
- [Install](#install)
- [Usage](#usage)
- [JSON output](#json-output)
- [How it works](#how-it-works)
- [Development](#development)
- [FAQ](#faq)
- [License](#license)

## Why claude-usage?

Claude subscription plans don't give you a number of tokens. They give you a **percentage of a rolling 5-hour window and of a 7-day window** (plus per-model weekly caps), and each account has its own. The only official places to see them are the claude.ai Usage page and `/usage` inside Claude Code, and both show one account at a time. With two or three logins, finding out which one can take a long session means logging into each in turn. claude-usage reads all of them at once.

- **All accounts, one view.** It finds `~/.claude` and every `~/.claude-*` config dir on its own, or you pass the ones you want.
- **Real plan percentages.** These are the server's numbers, not estimates from local logs, so usage from claude.ai, the desktop app, or another machine is included.
- **Read-only by design.** It never refreshes, rewrites, or copies a token, so it can't log your Claude Code sessions out.
- **Costs no quota.** Each account takes one GET request, with no model call.
- **Scriptable.** `--json` for `jq`, prompts, and status bars; exit codes you can branch on.
- **Zero dependencies.** One Go binary using only the standard library.

| | **claude-usage** | claude.ai Usage page | Claude Code `/usage` | Local log analyzers | Menu-bar apps |
|---|---|---|---|---|---|
| Real plan-limit percentages | ✅ | ✅ | ✅ | ❌ token / cost estimates | ✅ |
| Every account in one view | ✅ | ❌ one login | ❌ current account | ⚠️ merges logs, no limits | ⚠️ varies |
| Includes web, desktop and other machines | ✅ | ✅ | ✅ | ❌ this machine's logs | ✅ |
| Terminal and scriptable (`--json`) | ✅ | ❌ | ❌ | ✅ | ⚠️ varies |
| No GUI app or browser needed | ✅ | ❌ | ✅ | ✅ | ❌ |

If you want token and dollar breakdowns from your own logs, local log analyzers do that well. If you want to know how much of each account's plan is left, right now, from a terminal, that's claude-usage.

## Install

With Go 1.25 or newer:

```sh
go install github.com/khanakia/claude-usage@latest
```

This puts `claude-usage` in `$(go env GOPATH)/bin`, which needs to be on your `PATH`.

From source:

```sh
git clone https://github.com/khanakia/claude-usage.git
cd claude-usage
task install        # or: go install .
```

## Usage

```sh
claude-usage                         # every account found under ~/.claude and ~/.claude-*
claude-usage ~/.claude-work          # one or more specific config dirs
claude-usage --json                  # machine-readable output
claude-usage --color=never           # plain text (NO_COLOR is honoured too)
claude-usage --version
```

| Flag | Default | Meaning |
|---|---|---|
| `--json` | off | Print a JSON array instead of bars |
| `--color` | `auto` | `auto` (colour on a TTY unless `NO_COLOR` is set), `always`, or `never` |
| `--timeout` | `15s` | Per-account request timeout |
| `--version` | | Print the version and exit |

Exit status: `0` every account reported, `1` at least one account failed (the others are still printed), `2` usage error such as a bad flag, a missing directory, or no accounts found.

A config dir counts as an account when it contains `settings.json`, `projects/`, `.credentials.json`, or `.claude.json`, so look-alikes such as `~/.claude-worktrees` are skipped.

## JSON output

```sh
claude-usage --json | jq '.[] | {email, limits: [.limits[] | {label, percent, resets_at}]}'
```

Each array entry is one account:

```json
{
  "config_dir": "/Users/you/.claude-work",
  "email": "you@work.example",
  "organization": "Work",
  "plan": "Max (20x)",
  "limits": [
    { "label": "Current session", "kind": "session", "group": "session", "percent": 5, "severity": "normal", "resets_at": "2026-09-24T09:30:00Z", "scope": null, "is_active": true },
    { "label": "Fable", "kind": "weekly_scoped", "group": "weekly", "percent": 0, "severity": "normal", "resets_at": "2026-09-30T06:00:00Z", "scope": { "model": { "id": null, "display_name": "Fable" }, "surface": null }, "is_active": false }
  ],
  "extra_usage": { "is_enabled": false }
}
```

An account that failed has an `error` string instead of `limits`. `extra_usage` is passed through exactly as the server sent it.

## How it works

1. **Find accounts.** Claude Code keeps one login per config directory: `~/.claude` by default, or whatever `CLAUDE_CONFIG_DIR` points to.
2. **Read the login.** For each directory it reads the OAuth access token from `<dir>/.credentials.json` and from the macOS Keychain entry `Claude Code-credentials-<first 8 hex of sha256(dir)>` (plus the older unhashed `Claude Code-credentials` entry for `~/.claude`). When both exist, it uses the one that expires later. The email and organization come from `.claude.json`.
3. **Ask for usage.** It sends `GET https://api.anthropic.com/api/oauth/usage` with that token. This is the endpoint Claude Code's `/usage` uses.
4. **Render.** Each limit row becomes a bar with its percent used and time until reset.

The token is never refreshed. Refreshing replaces the saved login and could log the matching Claude Code install out. If a token has expired, claude-usage tells you the exact command to run (for example ``CLAUDE_CONFIG_DIR=~/.claude-work claude``) so that Claude Code refreshes it itself.

## Development

```sh
task check      # gofmt, go vet, staticcheck, go test -race, build
task test       # tests only
task run -- --json
```

The code is split into `internal/account` (discovering config dirs and resolving credentials; no network) and `internal/usage` (fetching, parsing and rendering; no knowledge of where tokens come from), with `main.go` connecting the two. See [CONTRIBUTING.md](CONTRIBUTING.md).

## FAQ

**Can I see how many tokens I have left on my Claude plan?**

No tool can. Pro and Max plans have no fixed token budget, and the server only reports the percentage of each window used. claude-usage shows that percentage for every account, which is the most precise figure that exists.

**Does checking usage use up my quota?**

No. It makes one read-only GET per account and never calls a model.

**Does it send my token or data anywhere?**

Only to `api.anthropic.com`, the same place Claude Code sends it. There is no telemetry and no other network call.

**Will it log me out of Claude Code?**

No. It only reads the stored login and never refreshes or rewrites it. An expired token is reported rather than refreshed.

**How do I track multiple Claude accounts?**

Give each account its own config dir by starting Claude Code with `CLAUDE_CONFIG_DIR=~/.claude-work claude`, log in once, and claude-usage picks that dir up automatically.

**Does it work with an API key (Console) account?**

No. API-key accounts are billed per token and have no plan limits. claude-usage reports that it found no OAuth login for such a dir.

**Does it work on Linux or Windows?**

On Linux, Claude Code stores the login in `.credentials.json`, which claude-usage reads, so it works. The Keychain lookup is macOS-only and is skipped elsewhere. Windows is untested.

**Is this an official Anthropic tool?**

No. It is an independent open-source project and is not affiliated with Anthropic. The usage endpoint is undocumented and could change; the parser falls back to the older response fields and never hides a limit it doesn't recognise.

## License

[Apache-2.0](LICENSE) © 2026 khanakia

<sub>claude-usage: open-source Claude usage tracker CLI for Claude Code, Claude Pro and Claude Max. Checks 5-hour session and weekly rate limits across multiple accounts (CLAUDE_CONFIG_DIR). Local, read-only, no telemetry, written in Go.</sub>
