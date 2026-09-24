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
- [Examples](#examples)
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

## Examples

Copy-paste recipes for every command. Output shown is trimmed; emails are placeholders.

### Check usage

```sh
agx                                  # every account, every provider (same as `agx usage`)
agx usage work                       # one profile
agx usage work personal              # several profiles, in this order
agx usage --provider claude          # only Claude accounts
agx usage --provider codex           # only Codex accounts
agx usage --color=never | less       # plain text, e.g. for a pager or a log
agx usage --timeout 5s               # give up on a slow account sooner
```

```text
$ agx usage work
you@work.example  Max (20x)  work · claude  ~/.claude-work
  Current session    ███░░░░░░░░░░░░░░░░░░░░░░░░░░░    9% used  resets in 3 hr 33 min
  Weekly limits
  All models         █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets in 6 d 0 hr
```

### Script usage with `--json` and `jq`

```sh
# one line per account: its fullest window
agx usage --json | jq -r '.data[] | "\(.profile): \([.windows[]?.percent] | max // 0)%"'
# personal: 79%
# work: 15%
# codex: 10%

# accounts that still have room (every window under 50%)
agx usage --json | jq -r '.data[] | select(([.windows[]?.percent] | max // 100) < 50) | .profile'

# compact one-liner for a status bar or tmux
agx usage --json --provider claude | jq -r '[.data[] | "\(.profile) \([.windows[]?.percent] | max // 0 | floor)%"] | join(" · ")'
# personal 79% · work 15%

# when each account's 5-hour session window resets
agx usage --json | jq -r '.data[] | .profile as $p | .windows[]? | select(.kind=="session") | "\($p) resets \(.resets_at)"'

# only the accounts that failed (expired login, rate limit, …)
agx usage --json | jq -r '.data[] | select(.error) | "\(.profile): \(.error)"'

# branch on the exit code: 0 all ok, 1 some account failed, 2 bad usage
agx usage >/dev/null || echo "at least one account needs attention"
```

### See your accounts

```sh
agx profiles                         # table: profile, provider, home, account, login, billing, source
agx who                              # same thing (alias; also `agx accounts`)
agx profiles --json | jq -r '.data[] | "\(.name)\t\(.login)\t\(.email // "-")"'
```

```text
$ agx profiles
PROFILE    PROVIDER  HOME            ACCOUNT                         LOGIN             BILLING  SOURCE
*personal  claude    ~/.claude       you@example.com · Max (20x)     ok (3 hr 38 min)  plan     config
work       claude    ~/.claude-work  you@work.example · Max (20x)    ok (6 hr 37 min)  plan     config
kimi       claude    ~/.claude       you@example.com · Max (20x)     ok (3 hr 38 min)  api      config
*codex     codex     ~/.codex                                        ok                plan     config
```

`*` marks each provider's default profile — the one `run` / `new` use when you pass no `-p`.

### Start an agent here: `agx run`

```sh
agx run                              # default Claude profile, in the current folder
agx run -p work                      # the work account
agx run -p auto                      # the Claude account with the most headroom
agx run -p codex                     # Codex
agx run -p auto --provider codex     # the Codex account with the most headroom
agx run -p kimi                      # Claude Code on Kimi (key fetched from gopass at launch)
agx run -p work -- --model sonnet    # everything after -- goes to the agent
agx run -p work -- -c                # e.g. continue Claude's most recent conversation here
agx run -p work --dry-run            # print what would run; starts nothing
```

```text
$ agx run -p work --dry-run
profile: work (claude, /Users/you/.claude-work)
dir:     /Users/you/code/app
env:     CLAUDE_CONFIG_DIR=/Users/you/.claude-work
exec:    claude --dangerously-skip-permissions

$ agx run -p auto
agx: auto → work (max 9%) over personal (max 78%)
```

### Start in a fresh session folder: `agx new`

```sh
agx new                              # ~/…/sessions/20260924_101500, default profile
agx new pglite spike                 # ~/…/sessions/pglite_spike_20260924_101500
agx new -p work invoice bug          # on the work account
agx new -p auto                      # on whichever Claude account has more room
agx new -p codex refactor            # a Codex session
agx new -p work demo --dry-run       # show the folder and command; create nothing
```

With the [shell layer](#shell-integration) loaded, your shell is left inside the new folder when the agent exits.

### Pick up where you left off: `agx resume`

```sh
agx resume                           # conversations started in THIS folder → fzf picker
agx resume pglite                    # filter by title, folder or id (all words must match)
agx resume --last                    # newest match, no picker
agx resume --all                     # search every folder, not just this one
agx resume --all --last invoice      # newest conversation anywhere mentioning "invoice"
agx resume --list                    # print instead of launching
agx resume --all --list --limit 20   # the 20 most recent conversations anywhere
agx resume --last --dry-run          # show which account / folder / id it would use
agx resume -p personal --last        # force a profile (normally chosen automatically)
agx resume --all --list --json | jq -r '.data[] | "\(.updated[0:16])  \(.provider)  \(.title // "(untitled)")"'
```

```text
$ agx resume --all --list --limit 3
just now     claude   Claude account usage limits          ~/work/sessions/20260923_175923   2bec3f80-…
23 min ago   claude   Automate visa application form        ~/work/sessions/visa_20260901_…   5c9565db-…
1 hr ago     codex    (untitled)                           ~/work/agentop                    019eea1d-…

$ agx resume --last --dry-run         # in a folder whose conversation lives on the work account
profile: work (claude, /Users/you/.claude-work)
dir:     /Users/you/work/sessions/20260921_091439
exec:    claude --dangerously-skip-permissions --resume dbcaeaf2-…
```

The account is picked for you: a conversation stored in `~/.claude-work` resumes on `work`, one in `~/.codex` resumes with `codex resume`.

### Session folders: `agx sessions`

```sh
agx sessions ls                      # every folder: last activity, file count, has history?
agx sessions ls --json | jq -r '.data[] | select(.files==0) | .name'   # folders with no files

agx sessions gc                      # show which empty, history-less folders would go
agx sessions gc --yes                # actually remove them
agx sessions gc --min-age 72h --yes  # only ones older than 3 days

agx sessions promote pglite_spike_20260924_101500 pglite-go             # → <promote_root>/pglite-go
agx sessions promote . my-tool                  # promote the folder you are in
agx sessions promote . my-tool --git-init       # …and git init it
agx sessions promote . my-tool --to ~/code      # choose the destination parent
agx sessions promote . my-tool --dry-run        # show the plan; change nothing
```

```text
$ agx sessions promote . my-tool --dry-run
would move ~/work/sessions/20260923_175923 → ~/work/my-tool
  history in ~/.claude would move with it
```

**What promote does, step by step** (`agx sessions promote pglite_spike_20260924_101500 pglite-go`):

1. Finds the folder: a bare name is looked up under `sessions.root`; `.` is the folder you are in; anything else is a path.
2. Checks every account's history (Claude and Codex, all homes) for conversations started in that folder.
3. Moves the folder to `<promote_root>/pglite-go` with a single rename, refusing if the destination exists.
4. **Claude:** Claude Code files each folder's conversations under `~/.claude*/projects/<folder path with punctuation as ->`. Promote renames that history folder to match the new path in whichever account holds it, so **starting Claude in the new folder continues the same chats**: `claude -c`, `claude --resume`, `agx resume`, `clr`.
5. **Codex:** Codex files conversations by date, not by folder, so there is nothing to move. The conversation stays resumable by id (`agx resume --all <words>`), but it still names the old folder, so after promoting a folder with Codex chats, `cd` into the new folder and start a new Codex session or resume by id from there.
6. Optionally runs `git init` (`--git-init`).

The destination must be on the same disk as the session folder — the move is one atomic rename, so it never leaves a half-copied project. Across disks promote stops with "destination is on a different disk" and changes nothing; use `mv` yourself in that case.

```sh
# after promoting:
cd ~/work/pglite-go
agx resume --last          # continues the chat that started in the session folder
claude -c                  # Claude's own "continue" finds it too
```

### Health check: `agx doctor`

```sh
agx doctor
agx doctor --json | jq -r '.data[] | select(.level=="warn" or .level=="fail") | "\(.level) \(.area): \(.detail)"'
```

```text
✓ config             ~/.agx/config.yaml (decided by home-dotfile)
✓ sessions.root      ~/work/sessions
✓ profile personal   you@example.com Max (20x)
✓ profile work       you@work.example Max (20x)
✓ profile kimi       api-billed (secrets: [ANTHROPIC_AUTH_TOKEN])
✓ profile codex      logged in
✓ shell              shell layer active (zsh)
```

### Shell setup, completion and version

```sh
echo 'eval "$(agx shell-init zsh)"' >> ~/.zshrc      # zsh
echo 'eval "$(agx shell-init bash)"' >> ~/.bashrc    # bash
agx shell-init zsh                                   # just look at what it defines

agx completion zsh > "${fpath[1]}/_agx"              # zsh tab completion (restart the shell)
agx completion bash > "$(brew --prefix)/etc/bash_completion.d/agx"   # Homebrew bash-completion

agx version                                          # version, commit, build time
agx version --json
agx --version
```

### Config recipes

Two Claude accounts, nothing else:

```yaml
# ~/.agx/config.yaml
profiles:
  - { name: personal, provider: claude, home: ~/.claude, default: true }
  - { name: work,     provider: claude, home: ~/.claude-work }
```

Add a third Claude account (log in once with `CLAUDE_CONFIG_DIR=~/.claude-client claude`, then):

```yaml
  - { name: client, provider: claude, home: ~/.claude-client }
```

A second Codex account (log in once with `CODEX_HOME=~/.codex-work codex`, then):

```yaml
  - { name: codex-work, provider: codex, home: ~/.codex-work }
```

Flags for every Claude launch, and your session folder:

```yaml
sessions:
  root: ~/work/sessions
  promote_root: ~/work
providers:
  claude:
    args: [--dangerously-skip-permissions]
```

A third-party backend billed per token (Kimi shown), with its key in gopass or in another env var:

```yaml
  - name: kimi
    provider: claude
    home: ~/.claude
    billing: api
    env:
      ANTHROPIC_BASE_URL: https://api.moonshot.ai/anthropic
      ANTHROPIC_MODEL: kimi-k3
    secrets:
      ANTHROPIC_AUTH_TOKEN: gopass:personal/ai/moonshot     # or: env:MOONSHOT_API_KEY
```

Short aliases (generated by `agx shell-init`):

```yaml
shell:
  aliases:
    cl: run -p personal
    clw: run -p work
    cla: run -p auto
    clnew: new -p personal
    clwnew: new -p work
    clanew: new -p auto
    clr: resume
    clwho: profiles
```

Then `clw`, `clanew spike`, `clr pglite`, … work like the commands they stand for, and extra words are passed through (`clw -- --model sonnet`).

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
| `agx completion zsh\|bash\|fish` | Shell tab-completion script |

### Choosing a profile: `-p`

`run`, `new` and `resume` take `-p` / `--profile`. The value is a profile name from `agx profiles` (e.g. `personal`, `work`, `kimi`, `codex`) or `auto`:

```sh
agx run                   # the default Claude profile (marked * in `agx profiles`)
agx run -p work           # a named profile
agx run -p auto           # the Claude account with the most headroom
agx run -p codex          # any profile, any provider
agx new -p auto --provider codex my spike   # auto-pick among Codex accounts
```

`--provider` (default `claude`) only matters when no profile is named or `-p auto` is used: it says which provider's default / auto-pick to use. `resume` normally needs no `-p` — it resumes on the account whose home stores the conversation — and `-p` there is an override.

### Flags by command

| Command | Flags |
|---|---|
| `agx` / `agx usage` | `--provider claude\|codex` · `--json` · `--color auto\|always\|never` · `--timeout 15s` |
| `agx profiles` | `--json` (aliases: `agx who`, `agx accounts`) |
| `agx run` | `-p/--profile` · `--provider` · `--dry-run` · `-- <args passed to the agent>` |
| `agx new` | `-p/--profile` · `--provider` · `--dry-run` · `[slug words]` |
| `agx resume` | `[query words]` · `--last` · `--all` · `--list` · `--json` (with `--list`) · `--limit 50` · `-p/--profile` · `--dry-run` |
| `agx sessions ls` | `--json` |
| `agx sessions gc` | `--yes` · `--min-age 24h` · `--json` |
| `agx sessions promote <folder> <name>` | `--to <parent>` · `--dry-run` · `--git-init` · `--json` |
| `agx doctor` | `--json` |
| `agx shell-init` | `zsh` (default) or `bash` |
| `agx version` / `agx --version` | `--json` (on `version`) |

`run`, `new` and `resume` accept `--dry-run` to print the exact command, working folder and environment changes without starting anything; secret values are shown as `<secret:scheme>`. Every command has `--help` with examples. Exit status: `0` ok, `1` a runtime failure (including "some accounts failed", with the others still printed), `2` a usage error.

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

See [CONTRIBUTING.md](CONTRIBUTING.md) for the dev loop and conventions.

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

**After `agx sessions promote`, does Claude still have my conversation in the new folder?**

Yes. Promote moves the Claude history along with the folder, so `claude -c`, `claude --resume` and `agx resume` in the new folder continue the same chat (verified against a live Claude Code 2.1.281 session). Codex conversations are not tied to a folder and stay resumable by id.

**Can I run `agx` on every shell prompt or in a tight loop?**

Don't: the vendors rate-limit their usage endpoints, and polling many times a minute gets an account answered with HTTP 429. agx then shows "usage endpoint rate-limited this account; try again shortly (retry after …)" for that account and keeps showing the others. Every few minutes (for example `watch -n 300 agx`) is fine.

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
