<h1 align="center">agx</h1>
<p align="center"><strong>One CLI for your AI coding agents and all their accounts.</strong></p>
<p align="center">See Claude Code and Codex plan usage for every account at once, launch on the account with the most headroom, resume any conversation on the account that holds it, and keep your session folders tidy.</p>

<p align="center">
  <a href="https://github.com/khanakia/agx/releases"><img src="https://img.shields.io/github/v/release/khanakia/agx?include_prereleases&color=2563eb" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-2563eb" alt="License: Apache-2.0"></a>
  <img src="https://img.shields.io/badge/go-%E2%89%A51.26-2563eb?logo=go&logoColor=white" alt="Go 1.26 or newer">
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-2563eb" alt="Platforms: macOS and Linux">
  <img src="https://img.shields.io/badge/agents-Claude%20Code%20%7C%20Codex-2563eb" alt="Agents: Claude Code and Codex">
  <a href="https://github.com/khanakia/agx/stargazers"><img src="https://img.shields.io/github/stars/khanakia/agx?style=flat&color=2563eb" alt="GitHub stars"></a>
</p>

**agx** is an open-source command-line toolkit for AI coding agents. If you run [Claude Code](https://docs.anthropic.com/en/docs/claude-code) with more than one account (personal and work logins via `CLAUDE_CONFIG_DIR`), or use Claude Code alongside OpenAI's Codex CLI, agx puts them behind one command: plan usage for every account in one view (the same numbers as claude.ai Settings → Usage and ChatGPT's Codex limits), `agx run -p auto` to start on whichever account has the most room left, `agx resume` to continue a past conversation on the account that actually stores it, and `agx new` / `agx sessions` for throwaway session folders. It is a single Go binary, reads only the logins your agent CLIs already saved, and never refreshes or copies a token.

```text
$ agx
you@example.com  Max (20x)  personal · claude  ~/.claude
  Current session    █████░░░░░░░░░░░░░░░░░░░░░░░░░   15% used  resets in 3 hr 33 min
  Weekly limits
  All models         ███████████████████████░░░░░░░   78% used  resets in 12 hr 3 min
  Fable              █████████████████████░░░░░░░░░   70% used  resets in 12 hr 3 min

you@work.example  Max (20x)  work · claude  ~/.claude-work
  Current session    ███░░░░░░░░░░░░░░░░░░░░░░░░░░░    9% used  resets in 3 hr 33 min
  Weekly limits
  All models         █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets in 6 d 0 hr
  Fable              ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    0% used  resets in 6 d 0 hr

you@example.com  Free  codex · codex  ~/.codex
  30-day             ███░░░░░░░░░░░░░░░░░░░░░░░░░░░    9% used  resets in 9 d 3 hr

$ agx run -p auto
agx: auto → work (max 9%) over personal (max 78%)
```

## Contents

- [Why agx?](#why-agx)
- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [Configuration](#configuration)
- [Shell integration](#shell-integration)
- [JSON output](#json-output)
- [How it works](#how-it-works)
- [Use it as a library](#use-it-as-a-library)
- [Development](#development)
- [FAQ](#faq)
- [License](#license)

## Why agx?

Claude and Codex subscriptions don't give you a token balance. They give you a **percentage of rolling windows** (5-hour, weekly, per model), per account. With two or three logins you end up checking each account's usage page by hand, starting sessions on the account that is nearly out, and — because each Claude config dir keeps its own history — trying to resume a work conversation from your personal login and finding nothing. agx fixes the whole loop.

- **Every account, every provider, one view.** Claude Code and Codex, all config dirs, found automatically.
- **Launch where there is room.** `-p auto` scores each account by its fullest window and picks the emptiest, explaining the choice.
- **Resume on the right account.** agx knows which home stores each conversation, finds it by folder or title, and resumes it there.
- **Session folders that clean up after themselves.** `agx new` makes a timestamped folder; `agx sessions gc` removes the empty ones that have no history; `agx sessions promote` turns one into a project and moves its Claude history with it.
- **Your shell habits, kept.** `agx shell-init` generates your old `cl` / `clw` / `clnew` style aliases from config, and lets `new` / `resume` leave your shell in the session folder.
- **Read-only and quota-free.** One GET per account to the vendor's own usage endpoint; tokens are never refreshed, written or printed.

| | **agx** | claude.ai / ChatGPT usage pages | Claude Code `/usage` | Local log analyzers | Hand-written shell aliases |
|---|---|---|---|---|---|
| Real plan-limit percentages | ✅ | ✅ | ✅ | ❌ token / cost estimates | ❌ |
| Every account and provider in one view | ✅ | ❌ one login each | ❌ current account | ⚠️ merges logs, no limits | ❌ |
| Launch on the account with the most headroom | ✅ | ❌ | ❌ | ❌ | ❌ |
| Resume a conversation on the account that holds it | ✅ | ❌ | ⚠️ current account only | ❌ | ⚠️ if you remember which |
| Session folder cleanup / promote with history | ✅ | ❌ | ❌ | ❌ | ⚠️ manual |
| Scriptable (`--json`) | ✅ | ❌ | ❌ | ✅ | ⚠️ varies |

If you want token and dollar breakdowns from your own logs, local log analyzers do that well. If you want to know which account can take the next session, start it there, and pick it back up later without thinking about accounts, that's agx.

## Install

With Go 1.26 or newer:

```sh
go install github.com/khanakia/agx@latest
```

This puts `agx` in `$(go env GOPATH)/bin`, which needs to be on your `PATH`. From source: `git clone https://github.com/khanakia/agx.git && cd agx && task install`.

## Quick start

```sh
agx                      # usage for every account (same as `agx usage`)
agx profiles             # which accounts agx found, and their login state
agx doctor               # check config, logins, binaries, shell integration
agx run -p auto          # start Claude Code on the account with the most headroom
agx new pglite spike     # new session folder pglite_spike_YYYYMMDD_HHMMSS, start there
agx resume               # pick a conversation from this folder and continue it
```

No config is needed: agx discovers `~/.claude` (profile `personal`), every `~/.claude-<name>` (profile `<name>`), and `~/.codex` (profile `codex`). Add a config file when you want named profiles, extra flags, secrets or aliases.

## Commands

| Command | Does |
|---|---|
| `agx` / `agx usage [profile…]` | Plan usage bars for every plan-billed login; `--provider`, `--json`, `--color`, `--timeout` |
| `agx profiles` | Every profile: provider, home, account, plan, login state, billing, source |
| `agx run [-p profile\|auto] [-- args…]` | Start the agent in the current folder with the profile's flags, env and secrets |
| `agx new [-p profile\|auto] [slug…]` | Create `sessions.root/[slug_]YYYYMMDD_HHMMSS`, enter it, start the agent |
| `agx resume [query…]` | Pick a conversation (this folder first, else recent everywhere) and continue it on its own account; `--last`, `--all`, `--list`, `--json` |
| `agx sessions ls` | Session folders with file counts and whether any provider holds history for them |
| `agx sessions gc [--yes]` | Remove empty session folders with no history (dry run unless `--yes`) |
| `agx sessions promote <folder> <name>` | Move a session folder into a project, taking its Claude history along; `--dry-run`, `--git-init` |
| `agx doctor` | Local health checks (no network, never resolves secret values) |
| `agx shell-init [zsh\|bash]` | Print the shell layer (see below) |
| `agx version` | Version, commit and build provenance |

`run`, `new` and `resume` accept `--dry-run` to print the exact command, working folder and environment changes without starting anything; secret values are shown as `<secret:scheme>`. Exit status: `0` ok, `1` a runtime failure (including "some accounts failed", with the others still printed), `2` a usage error.

## Configuration

Optional, at `~/.agx/config.yaml` (override the directory with `AGX_CONFIG_DIR` or `AGX_HOME`; `agx doctor` shows which one decided). Unknown keys are an error, so typos are caught.

```yaml
sessions:
  root: ~/work/sessions             # where `agx new` creates folders (default ~/agx-sessions)
  promote_root: ~/work              # default target of `sessions promote`

providers:
  claude:
    args: [--dangerously-skip-permissions]   # added to every Claude profile

profiles:                           # order breaks auto-pick ties
  - name: personal
    provider: claude
    home: ~/.claude
    default: true
  - name: work
    provider: claude
    home: ~/.claude-work
  - name: kimi                      # Claude Code on another backend, billed per token
    provider: claude
    home: ~/.claude
    billing: api                    # no usage windows; never chosen by auto
    args_replace: true              # use only these args
    args: [--dangerously-skip-permissions]
    env:
      ANTHROPIC_BASE_URL: https://api.moonshot.ai/anthropic
    secrets:                        # resolved at launch, never stored
      ANTHROPIC_AUTH_TOKEN: gopass:personal/ai/moonshot
  - name: codex
    provider: codex
    home: ~/.codex

shell:
  aliases:
    cl: run -p personal
    clw: run -p work
    cla: run -p auto
    clnew: new -p personal
    clr: resume
```

Secrets use `gopass:<path>` (runs `gopass show -o`) or `env:<NAME>`. When a provider has any configured profile, discovery for that provider is switched off, so the config is the complete list.

## Shell integration

A program cannot change its parent shell's directory, so add this to `~/.zshrc` (or `~/.bashrc`):

```sh
eval "$(agx shell-init zsh)"
```

It defines an `agx` function: for `new` and `resume`, the binary writes a short-lived launch plan (never containing secrets), the function `cd`s into the session folder, reports it to your terminal (OSC 7) so new tabs open there, and then starts the agent. It also defines one function per `shell.aliases` entry, so `cl`, `clw`, `clnew` and friends keep working. Without it, every command still works; your shell just stays where it was.

## JSON output

Every listing command has `--json`, emitting a stable envelope:

```json
{ "schema_version": 1, "kind": "usage.list", "count": 3, "data": [ … ] }
```

Kinds: `usage.list`, `profile.list`, `conversation.list`, `session.list`, `session.gc`, `session.promote`, `doctor.report`, `version.show`.

```sh
agx usage --json | jq '.data[] | {profile, windows: [.windows[] | {label, percent}]}'
agx resume --all --list --json | jq -r '.data[] | "\(.updated)  \(.title)"'
```

## How it works

| | Claude Code | Codex |
|---|---|---|
| Accounts | one per config dir: `~/.claude`, or `CLAUDE_CONFIG_DIR` (`~/.claude-work`, …) | one per `CODEX_HOME` (default `~/.codex`) |
| Login read from | `<home>/.credentials.json` and the macOS Keychain entry `Claude Code-credentials-<sha256(home)[:8]>` (freshest wins) | `<home>/auth.json` |
| Usage from | `GET api.anthropic.com/api/oauth/usage` (what `/usage` calls) | `GET chatgpt.com/backend-api/wham/usage` |
| Conversations | `<home>/projects/<path with non-alphanumerics as ->/<id>.jsonl`; titles from `ai-title` / `custom-title` records | `<home>/sessions/YYYY/MM/DD/rollout-*.jsonl` |
| Resume | `claude --resume <id>` in the conversation's folder | `codex resume <id>` |

agx never refreshes a token: refreshing replaces the saved login and can log the agent CLI out. An expired login is reported with the command that makes the CLI refresh it itself. When launching on the default home, agx *removes* `CLAUDE_CONFIG_DIR` / `CODEX_HOME` from the environment, because setting Claude's variable even to `~/.claude` switches it to a different Keychain entry and the account looks logged out. Transcripts are never read whole: the folder comes from the start of each file and the title from its end, so `resume` stays fast even with gigabytes of history.

## Use it as a library

The building blocks are public, dependency-light packages:

| Package | What it gives you |
|---|---|
| `github.com/khanakia/agx/provider` | the vendor-neutral model (profiles, usage windows, conversations, launch commands) — standard library only |
| `github.com/khanakia/agx/provider/claude` | read Claude Code logins, usage, conversations; build launch / resume commands |
| `github.com/khanakia/agx/provider/codex` | the same for Codex |
| `github.com/khanakia/agx/config` | load and validate the YAML config against any set of providers |
| `github.com/khanakia/agx/pick` | the auto-pick policy (pure) |
| `github.com/khanakia/agx/sessions` | session folder naming, listing, safe gc, promote |
| `github.com/khanakia/agx/secret` | `scheme:ref` secret resolution (gopass, env) |

A test in `internal/archtest` enforces the layering (the domain model imports only the standard library; libraries never import the CLI framework).

## Development

```sh
task check      # gofmt, go vet, staticcheck, race tests, cross-compile for every platform, build
task test       # tests only
task run -- resume --list
```

The design lives in [`docsi/AGX_SPEC.md`](docsi/AGX_SPEC.md). See [CONTRIBUTING.md](CONTRIBUTING.md).

## FAQ

**Can I see how many tokens I have left on my Claude or Codex plan?**

No tool can. Subscription plans have no fixed token budget, and the servers only report the percentage of each window used. agx shows that percentage for every account, which is the most precise figure that exists.

**Does checking usage use up my quota?**

No. It makes one read-only GET per account and never calls a model.

**Does it send my token or data anywhere?**

Only to the vendor's own API (`api.anthropic.com` for Claude, `chatgpt.com` for Codex), the same place the agent CLI sends it. There is no telemetry.

**Will it log me out of Claude Code or Codex?**

No. It only reads the stored login and never refreshes or rewrites it. An expired login is reported, not refreshed.

**How do I add a second Claude account?**

Start Claude Code with `CLAUDE_CONFIG_DIR=~/.claude-work claude`, log in once, and agx discovers the `~/.claude-work` home as profile `work`.

**How does `-p auto` choose?**

It fetches usage for each plan-billed account of the provider, scores each by its fullest window (because any one window at 100% blocks you), and picks the lowest score; ties go to config order. API-billed profiles are never picked.

**Is it safe to run `agx sessions gc --yes`?**

It only removes folders that contain no files, have no conversation history in any provider, and are older than `--min-age` (24h by default), and deletion uses a call that cannot remove a non-empty directory.

**Does it work with API-key or third-party backends like Kimi?**

Yes, as a `billing: api` profile with its own `env` and `secrets`. Those launch normally and show "billed per token" in usage.

**Does it work on Linux or Windows?**

On Linux, logins are read from the credential files, so it works. The Keychain lookup is macOS-only and is skipped elsewhere. It compiles for Windows, where launching runs the agent as a child process; Windows is otherwise untested.

**Is this an official Anthropic or OpenAI tool?**

No. It is an independent open-source project, not affiliated with Anthropic or OpenAI. The usage endpoints are undocumented and could change; parsing is defensive and never hides a limit it doesn't recognise.

## License

[Apache-2.0](LICENSE) © 2026 khanakia

<sub>agx: open-source CLI toolkit for AI coding agents — Claude Code and OpenAI Codex usage limits across multiple accounts, auto account selection, cross-account conversation resume, session folder management. Local, read-only, no telemetry, written in Go.</sub>
