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

**agx** is an open-source command-line toolkit for AI coding agents. If you run [Claude Code](https://docs.anthropic.com/en/docs/claude-code) with more than one account (personal and work logins via `CLAUDE_CONFIG_DIR`), or use Claude Code alongside OpenAI's Codex CLI, agx puts them behind one command: plan usage for every account in one view (the same numbers as claude.ai Settings → Usage and ChatGPT's Codex limits), `agx run -p auto` to start on whichever account has the most room left, `agx resume` to continue a past conversation on the account that actually stores it, and `agx new` / `agx sessions` for throwaway session folders. It is a single Go binary that reads the logins your agent CLIs already saved and never refreshes or copies them; with `agx login` any home can also run on another account's quota.

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
- **Resume on the right account.** agx knows which home stores each conversation, finds it by folder or title, and resumes it there. Forgot the folder? `agx ws` lists every folder with conversations, then its chats.
- **Session folders that clean up after themselves.** `agx new` makes a timestamped folder; `agx sessions gc` removes the empty ones that have no history; `agx sessions promote` turns one into a project and moves its Claude history with it.
- **Any history, any account's quota.** `agx login` hands a long-lived token to Claude Code at launch, so your work folder can spend your personal plan (or the other way round) without moving a single conversation.
- **Your shell habits, kept.** `agx shell-init` generates your old `cl` / `clw` / `clnew` style aliases from config, and lets `new` / `resume` / `ws` leave your shell in the session folder.
- **Read-only and quota-free.** One GET per account to the vendor's own usage endpoint; logins are never refreshed or printed, and the only token agx ever writes is one you add with `agx login add`, into a private 0600 file.

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

This puts `agx` in `$(go env GOPATH)/bin`, which needs to be on your `PATH`.

Prebuilt binaries (macOS, Linux, Windows; amd64 and arm64) are on the [Releases page](https://github.com/khanakia/agx/releases), with a `checksums.txt`:

```sh
# macOS on Apple Silicon — swap darwin_arm64 for darwin_amd64, linux_amd64, linux_arm64
curl -sL https://github.com/khanakia/agx/releases/download/v0.3.3/agx_v0.3.3_darwin_arm64.tar.gz | tar -xz agx
sudo mv agx /usr/local/bin/
agx version
```

From source: `git clone https://github.com/khanakia/agx.git && cd agx && task install`.

## Quick start

```sh
agx                      # usage for every account (same as `agx usage`)
agx profiles             # which accounts agx found, and their login state
agx doctor               # check config, logins, binaries, shell integration
agx run -p auto          # start Claude Code on the account with the most headroom
agx new pglite spike     # new session folder pglite_spike_YYYYMMDD_HHMMSS, start there
agx resume               # pick a conversation from this folder and continue it
agx ws                   # pick any folder with conversations, then a chat in it
agx run -p work --login personal   # work history, personal quota (after `agx login add personal`)
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

### Plan and subscription: `agx plan`

```sh
agx plan                             # every account: plan, subscription status, since when
agx plan personal                    # one profile
agx subscription                     # same command (alias)
agx plan --json | jq -r '.data[] | "\(.profile): \(.subscription.plan // "-") \(.subscription.status // "")"'
```

```text
$ agx plan
PROFILE   PROVIDER  ACCOUNT            PLAN       STATUS  SINCE
personal  claude    you@example.com    Max (20x)  active  2025-11-25
work      claude    you@work.example   Max (20x)  active  2026-04-23
codex     codex     —                  Free       —       —
```

The next renewal date is **not** shown: the Claude Code login agx reads can see the plan, status and start date (`/api/oauth/profile`) but not billing details — `api.anthropic.com` answers "This endpoint does not accept OAuth access tokens" for them. Find the next charge date on claude.ai → Settings → Billing.

### See your accounts

```sh
agx profiles                         # table: profile, provider, home, account, state, login, billing, source
agx who                              # same thing (alias; also `agx accounts`)
agx profiles --json | jq -r '.data[] | "\(.name)\t\(.login)\t\(.email // "-")"'
```

```text
$ agx profiles
PROFILE    PROVIDER  HOME            ACCOUNT                         STATE             LOGIN               BILLING  SOURCE
*personal  claude    ~/.claude       you@example.com · Max (20x)     ok (3 hr 38 min)  home                plan     config
work       claude    ~/.claude-work  you@work.example · Max (20x)    ok (6 hr 37 min)  personal (profile)  plan     config
kimi       claude    ~/.claude       you@example.com · Max (20x)     ok (3 hr 38 min)  home                api      config
*codex     codex     ~/.codex                                        ok                home                plan     config
```

`*` marks each provider's default profile — the one `run` / `new` use when you pass no `-p`. STATE is the home's own login (ok, expired, none); LOGIN is whose quota a run on that profile spends and which rule chose it (see [Logins](#logins--use-any-accounts-quota-from-any-home)). In `--json`, `login` is still the state and `login_name` / `login_source` are the login.

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
login:   home
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
agx ws                               # workspace picker: any folder with chats → its chats → resume
agx resume -b                        # the same (--browse)
agx ws invoice                       # only workspaces with a chat mentioning "invoice"
agx ws --list                        # print the workspaces (--json: kind workspace.list)
agx resume --all --list --json | jq -r '.data[] | "\(.updated[0:16])  \(.provider)  \(.profile)  \(.title // "(untitled)")"'
```

In a folder with no conversations, `agx resume` (and `--last`, `--list`) stops with `no conversations in <folder>` and a hint to use `--all`, exit 1; it never falls back to other folders on its own. `--list --json` prints an empty list with exit 0, so scripts still get valid JSON.

```text
$ agx resume --all --list --limit 3
just now     claude work       Claude account usage limits      …/20260923_175923             2bec3f80
23 min ago   claude personal   Automate visa application form   …/visa_20260901_194915        5c9565db
1 hr ago     codex  codex      (untitled)                       ~/work/agentop                019eea1d

$ agx resume --last --dry-run         # in a folder whose conversation lives on the work account
profile: work (claude, /Users/you/.claude-work)
dir:     /Users/you/work/sessions/20260921_091439
exec:    claude --dangerously-skip-permissions --resume dbcaeaf2-…
```

The account is picked for you: a conversation stored in `~/.claude-work` resumes on `work`, one in `~/.codex` resumes with `codex resume`. In `--list` and the picker, the provider (`claude` / `codex`) is followed by that profile. Each row stays on one line: long titles end in `…`, a folder that does not fit shows its last segment, and the id is cut to 8 characters, which still works as a query (`agx resume 2bec3f80`). When several profiles share one home (say `personal` and an api-billed `kimi` both on `~/.claude`), the transcript does not record which one made it, so the row shows the profile `resume` launches on (plan-billed first, then the default, then config order) and `--json` adds `profiles` with every candidate. `--json` keeps the full id, folder and `home`, plus `profile` and `profiles`.

**Don't remember which folder a chat was in?** `agx ws` (same as `agx resume --browse` / `-b`) lists every folder that has conversations, across all accounts and providers, newest activity first. Pick one, then pick a chat in it, and it resumes there on its own account. Esc on the chat list goes back to the folders; typing filters both lists. Folders that no longer exist are hidden. With the shell layer loaded, your shell is left in the chat's folder.

```text
$ agx ws --list
just now     8 chats   work,personal        ~/code/brick
2 min ago    3 chats   personal             ~/code/sqlb
17 d ago     4 chats   work                 ~/code/docker_setup_mac
```

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
3. Moves the folder to `<promote_root>/pglite-go` with [a single rename, refusing if the destination exists](ds:block?id=promote-bdf6x7wh).
4. **Claude:** Claude Code files each folder's conversations under `~/.claude*/projects/<folder path with punctuation as ->`. Promote renames that history folder to match the new path in whichever account holds it, so **starting Claude in the new folder continues the same chats**: `claude -c`, `claude --resume`, `agx resume`, `clr`.
5. **Codex:** Codex files conversations by date, not by folder, so there is nothing to move. The conversation stays resumable by id (`agx resume --all <words>`), but it still names the old folder, so after promoting a folder with Codex chats, `cd` into the new folder and start a new Codex session or resume by id from there.
6. Optionally runs `git init` (`--git-init`).

The destination must be on the same disk as the session folder — the move is one atomic rename, so it never leaves a half-copied project. Across disks promote [stops with "destination is on a different disk"](ds:block?id=promote-bdf6x7wh) and changes nothing; use `mv` yourself in that case.

```sh
# after promoting:
cd ~/work/pglite-go
agx resume --last          # continues the chat that started in the session folder
claude -c                  # Claude's own "continue" finds it too
```

### Switch a project to another account: `agx sessions move`

Worked in a folder on your personal account and want it on work from now on? Move its conversations; they then resume on work.

```sh
# from inside the project (the folder defaults to the current one)
cd ~/code/docker_setup_mac
agx sessions move --to work --dry-run          # show what would move
agx sessions move --to work                    # move this folder's conversations personal → work

# naming the folder instead
agx sessions move docker_setup_mac --to work   # works from ~/code or from inside docker_setup_mac
agx sessions move ~/code/docker_setup_mac --to work

# more control
agx sessions move . --to personal --from work  # move them back
agx sessions move . --to work --from personal  # only from this account (default: every other account)
agx sessions move . --to work --only aeba9843  # just one conversation (full id or unique prefix)
agx sessions move . --to work --only aeba9843,6144697a
agx sessions move . --to work --no-backup      # skip the backup copy
agx sessions move . --to work --json           # machine-readable plan / result

# pick instead of typing ids / folder names
agx sessions move -i                           # mark this folder's chats (Tab), then pick the target account
agx sessions move -i --to work --dry-run       # mark chats, show the plan
agx sessions move -i --all                     # pick the folder from every workspace first

# afterwards
agx resume --list                              # the conversations now list under the new account
clr                                            # pick one — it opens on work
```

```text
$ agx sessions move --to work --dry-run
would move 3 claude conversation(s) for ~/code/docker_setup_mac
  from ~/.claude → ~/.claude-work (work)
    33f18428-fc35-4b31-baeb-8e9f01576ff9  (untitled)
    6144697a-4e8c-4ef3-8caa-f5c0498e8530  (untitled)
    aeba9843-dbea-4d03-a715-7f83e009bf7c  Hatchet, ClickHouse, HyperDX services
  8 file(s)/folder(s) involved

$ agx sessions move --to work
moved 3 claude conversation(s) for ~/code/docker_setup_mac
  from ~/.claude → ~/.claude-work (work)
  …
  backup: ~/.claude/backups/agx-move-docker_setup_mac-20260924-203228
done — continue with: cd ~/code/docker_setup_mac && agx resume  (or claude -c on work)

$ agx sessions move --to work          # again: already done
no claude conversations for ~/code/docker_setup_mac outside work — nothing to move

$ agx sessions move --from personal --to codex
agx: can't move conversations from personal (claude) to codex (codex): providers store conversations in different formats — choose a --to profile of the same provider
```

How it works:

- The **target profile decides the provider** (`work` is a Claude profile, so Claude conversations move). Sources are every other account of that provider, or just `--from`.
- **Same provider only.** `--from personal --to codex` is [refused](ds:block?id=app-runmove-gs5yg4x2) — Claude and Codex store conversations in different formats. `personal → kimi` is refused too: they share `~/.claude`, so there is nothing to move.
- **Picking (`-i` / `--pick`):** lists this folder's conversations (provider, profile, title, id); mark one or more with Tab (Enter alone takes the highlighted one). Without `--to`, a second list offers the profiles of the same provider and marks the ones that hold the chosen chats now. The chosen chats become `--only`, and every check below still applies. In a folder with no conversations it stops with "use --all"; only `--all` opens the workspace list first. Without fzf, type numbers such as `1 3` or `1,3`. `-i` cannot be combined with a folder argument or `--only`.
- **Which folder:** no argument = the current folder. A bare name is looked up under `sessions.root`, then in the current folder, then matched against the current folder's own name (so `agx sessions move docker_setup_mac …` works from inside it too). Anything with a `/` is a path.
- **What moves (Claude):** each conversation's transcript and tool results, its file checkpoints (`file-history/`) and environment snapshot (`session-env/`), plus the folder's project memory. It merges into whatever the target already has; after a full move the now-empty history folder in the old account is removed (anything the move doesn't recognise is left in place and listed).
- **Safe by default:** it stops before changing anything if Claude is running in that folder (in either account) or a conversation already exists in the target; it copies the sources to `<home>/backups/agx-move-<folder>-<time>/` first (`--no-backup` to skip); and if a move fails half-way, [everything already moved is put back](ds:block?id=applymoveplan-j9u9m6j7). Running it twice is harmless — the second run finds nothing to move.
- **Exit status:** `0` moved (or nothing to move), `1` blocked or failed (nothing changed), `2` a wrong request (unknown profile, different providers, same home, missing folder).
- **Undo:** `agx sessions move --to personal --from work` moves them back; the backup folder holds the untouched originals.
- **Not moved:** Claude's global up-arrow prompt history and the folder's trust / allowed-tools settings — Claude asks to trust the folder once on the new account.
- **Codex:** not supported yet. Codex indexes conversations in its own SQLite database with absolute paths, so moving its files would break its index; agx says so instead of trying.

After moving: `agx resume` (or `clr`) in the folder opens the conversations on the new account, and `claude -c` works there under that account.

### Logins — use any account's quota from any home

A Claude home (`~/.claude`, `~/.claude-work`, …) holds two things: the conversation history and a login, so a work folder's history normally spends only work quota. A **login** is a named, long-lived token (from `claude setup-token`) or API key that agx hands to Claude Code when it launches. With it, any home runs on any account's quota, while its history, settings and `resume` stay exactly where they are.

```sh
# add a login; the token is never a command-line argument (argv shows in ps and shell history)
agx login add personal                                 # paste the token at a hidden prompt
pbpaste | agx login add personal --stdin               # or pipe it in
agx login add personal --setup                         # run `claude setup-token` first, then paste what it prints
agx login add work --cmd "gopass show -o personal/ai/claude-oauth-work"   # run at each launch, nothing stored
agx login add ci --env CLAUDE_WORK_TOKEN               # read from another env var at each launch
agx login add personal --stdin --account you@example.com   # a label, for your eyes
agx login add key --kind api-key --stdin               # an API key (billed per token) instead of plan quota

# one run
agx run -p work --login personal                       # work home and history, personal quota
clw --login personal                                   # every alias passes it through
agx resume --login personal                            # resume a work conversation on personal quota
agx run -p work --login home                           # ignore every default: the home's own login
agx run -p work --login personal --dry-run             # what would change; the token is never printed
agx run -p work --login auto                           # the login (or the home's own) with the most headroom

# a default
agx login use personal -p work                         # profile work always spends personal quota
agx login use personal --provider claude               # every Claude profile
aglogin personal                                       # this terminal only (shell function from `agx shell-init`)
aglogin                                                # undo for this terminal
agx login use home -p work                             # undo the profile default
agx login use home --provider claude                   # undo the provider default

# look after them
agx login ls                                           # NAME PROVIDER KIND SOURCE ACCOUNT ADDED EXPIRES USED-BY
agx login ls --json | jq -r '.data[] | "\(.name)\t\(.expires)"'
agx login check                                        # resolve every token and show its usage (one probe each)
agx login check --no-probe                             # only resolve; no request, no quota spent
agx usage --logins                                     # every home, then one probed row per login
agx usage --login personal                             # one login's 5-hour and weekly windows
agx login add personal --stdin --replace               # rotate a token (kind and account are kept)
agx login rm personal                                  # refused while a profile uses it; --force clears those too
```

```text
$ agx run -p work --login personal --dry-run
profile: work (claude, /Users/you/.claude-work)
login:   personal (--login)
dir:     /Users/you/code/app
env:     CLAUDE_CODE_OAUTH_TOKEN=<token:stored>
env:     CLAUDE_CONFIG_DIR=/Users/you/.claude-work
exec:    claude --dangerously-skip-permissions

$ agx run -p work --login personal
agx: work · login personal

$ agx run -p work --login auto
agx: login auto → personal (max 12%) over home (max 64%), spare (max 80%)
agx: work · login personal

$ agx login ls
NAME      PROVIDER  KIND         SOURCE  ACCOUNT          ADDED       EXPIRES      USED-BY
personal  claude    oauth-token  stored  you@example.com  2026-10-16  2027-10-16   work
work      claude    oauth-token  cmd     —                2026-10-16  ~2027-10-16  —
```

**Which login a run uses.** [The first match wins](ds:block?id=app-picklogin-p3tyusqh):

1. `--login NAME` on the command line (or `--login auto`, below);
2. `AGX_LOGIN` in the environment (what `aglogin` sets);
3. the profile's `login:` in config.yaml;
4. the provider's `providers.<id>.login:`;
5. the home's own login.

A login asked for by name (`--login`, or a profile's `login:`) that does not fit the profile is an error: another provider's login, a profile billed per token, or a profile that sets a credential of its own in `env:` / `secrets:`. One that comes from `AGX_LOGIN` is skipped with a one-line note and a provider default is skipped silently, so an exported Claude login never breaks a Codex or Kimi run. `agx profiles` shows the login each profile would use and why (`personal (profile)`, `work (AGX_LOGIN)`, `home`), and every launch on a login prints `agx: <profile> · login <name>`, which is the only record of which account paid: transcripts do not keep it.

**How it launches.** [An `oauth-token` login](ds:block?id=provider-loginenv-n8x9u9rf) sets `CLAUDE_CODE_OAUTH_TOKEN` and removes `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN` from the agent's environment (either would otherwise win and bill an API key); an `api-key` login sets `ANTHROPIC_API_KEY` and removes the other two. `CLAUDE_CONFIG_DIR` is handled exactly as without a login. A token that is missing, empty or more than one line, a `--cmd` that fails or runs longer than [10 seconds](ds:block?id=cmdtimeout-44pfsqzk), or an unset `--env` variable stops the launch with the reason. agx never falls back to the home's login on its own, because you asked for a specific account.

**Where tokens live.** Pasted and piped tokens go to `~/.agx/tokens.yaml`, beside config.yaml, [mode 0600 in a 0700 folder](ds:block?id=tokenfile-checkperm-4epx2b72), written atomically. config.yaml only says `token: stored`, so it stays safe to sync or share. tokens.yaml must never be synced, backed up to a shared place or committed: agx prints that warning whenever it stores a token, `agx doctor` warns when the file sits inside a git work tree, and agx refuses to read a tokens.yaml that group or others can access, the way ssh treats a private key. `--cmd` and `--env` logins store nothing; their command or variable is read at each launch.

**Usage of a login, and `--login auto`.** A setup-token cannot read the usage endpoint `agx usage` uses (it answers 403), so agx reads a login's usage with [a probe](ds:block?id=probeclient-probe-i9609s2l): one minimal request to the Messages API ([the smallest model](ds:block?id=defaultprobemodel-g7o7rdkd), one output token) whose response headers carry the account's 5-hour and weekly windows, the same numbers `agx usage` shows for a home. A probe spends a few tokens of that login's plan quota and counts toward its rate limits, so agx runs one only when you ask: `agx usage --logins` (one row per login after the homes; in `--json` those rows carry `login`, `account` and `"source": "probe"` instead of `profile` and `home`), `agx usage --login NAME` (that login only), `agx login check` (`--no-probe` only resolves the token) and `--login auto`. A plain `agx` or `agx usage` never probes. [`--login auto`](ds:block?id=app-autologin-a4w0xz3h) on `run`, `new`, `resume` and `ws` compares the home's own login with every login that fits the profile, using the same rule as `-p auto` (lowest fullest window wins; ties go to the home, then config order); a login whose token does not resolve or whose probe fails is left out with its reason, and if nothing is left the run stops. `auto` is accepted only as that flag: in config.yaml, `AGX_LOGIN` or `agx login use` it is refused, so probing never becomes a silent default. An api-key login has no plan windows and is never probed. `agx plan --login NAME` is refused: the plan comes from the profile endpoint, which also refuses setup-tokens.

**Expiry.** A setup-token is [believed to last one year](ds:block?id=setuptokenlifetime-nebd9cbt) (not verified yet) and is never refreshed. `agx login ls` shows EXPIRES counted from the login's `added` date, with `~` for `--cmd` and `--env` logins, where agx does not hold the value and `added` is when the login was added rather than when the token was minted. `agx doctor` warns [30 days](ds:block?id=expirywarnwindow-fnenahy6) ahead. Past that date [a stored login is refused](ds:block?id=app-resolveloginvalue-10wd6ob4) (doctor fails) with the command that renews it: `agx login add NAME --setup --replace`. A `--cmd` or `--env` login only warns and runs, and doctor warns instead of failing, because the token behind it may have been rotated in your store since the login was added.

**Good to know.**

- [Removing a login from agx does not revoke the token.](ds:block?id=revokehint-ngx5vax3) Revoke it where it was issued, in your claude.ai account settings.
- The agent receives the token in its environment, so any process it starts (tools, hooks, MCP servers) can read it. Use a login only with agents and projects you would trust with that account.
- Every probe (`agx usage --logins`, `--login NAME`, `--login auto`, `agx login check`) is a real, if tiny, request on that account: a few tokens of plan quota and one request toward its rate limits. Checking every few seconds in a loop would add up; `agx login add` does not probe.
- Codex has no logins yet: its plan login exists only as refreshable tokens in `auth.json` (copying them can log the CLI out), and which API-key variable interactive Codex honours is not verified.

What `agx login add` and `agx login use` write to config.yaml (comments and key order are kept):

```yaml
logins:
  - name: personal
    provider: claude              # default claude
    kind: oauth-token             # default; or api-key
    token: stored                 # the value is in tokens.yaml; nothing else is accepted here
    account: you@example.com      # a label
    added: 2026-10-16             # expiry counts from this
  - name: work
    provider: claude
    token_cmd: gopass show -o personal/ai/claude-oauth-work
    added: 2026-10-16
providers:
  claude:
    login: work                   # every Claude profile, unless the profile says otherwise
profiles:
  - { name: personal, provider: claude, home: ~/.claude, login: home }      # pinned to its own login
  - { name: work, provider: claude, home: ~/.claude-work, login: personal }
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
✓ tokens.yaml        ~/.agx/tokens.yaml (0600)
✓ login personal     oauth-token (stored), expires 2027-10-16
✓ login work         oauth-token (cmd), expires approximately 2027-10-16
✓ shell              shell layer active (zsh)
```

### Shell setup, completion and version

```sh
echo 'eval "$(agx shell-init zsh)"' >> ~/.zshrc      # zsh
echo 'eval "$(agx shell-init bash)"' >> ~/.bashrc    # bash
agx shell-init zsh                                   # just look at what it defines (aliases, aglogin)

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

Then `clw`, `clanew spike`, `clr pglite`, … work like the commands they stand for, and extra words are passed through (`clw -- --model sonnet`, `clw --login personal`).

A fixed account-and-quota pair is just an alias: `clwp: run -p work --login personal`.

## Commands

| Command | Does |
|---|---|
| `agx` / `agx usage [profile…]` | Plan usage bars for every plan-billed login; `--provider`, `--json`, `--color`, `--timeout` |
| `agx plan [profile…]` | Plan, subscription status and start date for every account; `--json`, `--timeout` |
| `agx profiles` | Every profile: provider, home, account, plan, login state, the login a run spends, billing, source |
| `agx run [-p profile\|auto] [-- args…]` | Start the agent in the current folder with the profile's flags, env and secrets |
| `agx new [-p profile\|auto] [slug…]` | Create `sessions.root/[slug_]YYYYMMDD_HHMMSS`, enter it, start the agent |
| `agx resume [query…]` | Pick a conversation (this folder, or every folder with `--all`) and continue it on its own account; `--last`, `--all`, `--list`, `--json`, `-b/--browse` |
| `agx ws [query…]` | Pick a workspace (any folder with conversations), then a chat in it, and resume it; `--list`, `--json` |
| `agx sessions ls` | Session folders with file counts and whether any provider holds history for them |
| `agx sessions gc [--yes]` | Remove empty session folders with no history (dry run unless `--yes`) |
| `agx sessions promote <folder> <name>` | Move a session folder into a project, taking its Claude history along; `--dry-run`, `--git-init` |
| `agx sessions move [folder] --to <profile>` | Move a folder's conversations to another account of the same provider (e.g. personal → work); `--from`, `--only`, `-i/--pick`, `--all`, `--dry-run`, `--no-backup`, `--json` |
| `agx login add\|ls\|check\|use\|rm` | Named logins: run any home on any account's quota ([Logins](#logins--use-any-accounts-quota-from-any-home)) |
| `agx doctor` | Local health checks (no network, never resolves secret or token values) |
| `agx shell-init [zsh\|bash]` | Print the shell layer (see below) |
| `agx version` | Version, commit and build provenance |
| `agx completion zsh\|bash\|fish` | Shell tab-completion script |

### Choosing a profile: `-p`

`run`, `new` and `resume` take `-p` / `--profile`, which picks the home (history, settings); `--login` picks whose quota it spends. The value is a profile name from `agx profiles` (e.g. `personal`, `work`, `kimi`, `codex`) or `auto`:

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
| `agx` / [`agx usage`](ds:block?id=newusagecmd-r3ert49b) | `--provider claude\|codex` · `--json` · `--color auto\|always\|never` · [`--timeout 15s`](ds:block?id=defaultusagetimeout-sz3xwdbj) · `--logins` · `--login NAME\|home` (both probe) |
| [`agx plan`](ds:block?id=newplancmd-urn3m4qd) | `[profile…]` · `--json` · [`--timeout 15s`](ds:block?id=defaultusagetimeout-sz3xwdbj) · `--login home` (alias: `agx subscription`) |
| [`agx profiles`](ds:block?id=newprofilescmd-ndr78yh5) | `--json` (aliases: `agx who`, `agx accounts`) |
| [`agx run`](ds:block?id=newruncmd-jgrurvce) | `-p/--profile` · `--provider` · `--login NAME\|home\|auto` · `--dry-run` · `-- <args passed to the agent>` |
| [`agx new`](ds:block?id=newnewcmd-u4uqk853) | `-p/--profile` · `--provider` · `--login NAME\|home\|auto` · `--dry-run` · `[slug words]` |
| [`agx resume`](ds:block?id=newresumecmd-wp72e3ak) | `[query words]` · `--last` · `--all` · `--list` · `--json` (with `--list`) · [`--limit 50`](ds:block?id=defaultresumelimit-zc5uhvc4) · `-b/--browse` · `-p/--profile` · `--login NAME\|home\|auto` · `--dry-run` |
| [`agx ws`](ds:block?id=newwscmd-s5rg592z) | `[query words]` · `--list` · `--json` (with `--list`) · `-p/--profile` · `--login NAME\|home\|auto` · `--dry-run` |
| [`agx sessions ls`](ds:block?id=newsessionslscmd-tv2vrc6k) | `--json` |
| [`agx sessions gc`](ds:block?id=newsessionsgccmd-qhhnyct9) | `--yes` · [`--min-age 24h`](ds:block?id=defaultgcminage-67k288nh) · `--json` |
| [`agx sessions promote`](ds:block?id=newsessionspromotecmd-tdxnub7p) `<folder> <name>` | `--to <parent>` · `--dry-run` · `--git-init` · `--json` |
| [`agx sessions move`](ds:block?id=newsessionsmovecmd-hpm92rna) `[folder]` | `--to <profile>` (required unless `-i` picks it) · `-i/--pick` · `--all` (with `-i`) · `--from <profile>` · `--only <ids>` · `--dry-run` · `--no-backup` · `--json` |
| [`agx login add`](ds:block?id=newlogincmd-zhxdkaat) `NAME` | `--stdin` · `--setup` · `--cmd <command>` · `--env <VAR>` · `--provider` · `--kind oauth-token\|api-key` · `--account <label>` · `--replace` |
| `agx login ls` | `--json` (alias: `agx login list`) |
| `agx login check` `[NAME]` | `--no-probe` |
| `agx login use` `NAME\|home` | `-p/--profile <profile>` or `--provider <id>` |
| `agx login rm` `NAME` | `--force` (alias: `agx login remove`) |
| [`agx doctor`](ds:block?id=newdoctorcmd-ndsdf4zg) | `--json` |
| [`agx shell-init`](ds:block?id=newshellinitcmd-3fw2m4vd) | `zsh` (default) or `bash` |
| `agx version` / `agx --version` | `--json` (on `version`) |

`run`, `new` and `resume` accept `--dry-run` to print the exact command, working folder and environment changes without starting anything; secret values are shown as `<secret:scheme>` and a login's token as `<token:source>`. Every command has `--help` with examples. [Exit status](ds:block?id=exitcodes-35ygnun6): `0` ok, `1` a runtime failure (including "some accounts failed", with the others still printed), `2` a usage error.

## Configuration

Optional, at `~/.agx/config.yaml` (override the directory with [`AGX_CONFIG_DIR` or `AGX_HOME`](ds:block?id=appidentity-65c6dy83); `agx doctor` shows which one decided). [Unknown keys are an error](ds:block?id=decode-qymvq95s), so typos are caught.

```yaml
sessions:
  root: ~/work/sessions             # where `agx new` creates folders (default ~/agx-sessions)
  promote_root: ~/work              # default target of `sessions promote`

logins:                             # named credentials; see Logins above
  - name: personal
    token: stored                   # the raw token is in ~/.agx/tokens.yaml
    added: 2026-10-16

providers:
  claude:
    args: [--dangerously-skip-permissions]   # added to every Claude profile
    # login: personal               # default login for every Claude profile

profiles:                           # order breaks auto-pick ties
  - name: personal
    provider: claude
    home: ~/.claude
    default: true
  - name: work
    provider: claude
    home: ~/.claude-work
    login: personal                 # runs here spend personal quota (or `home`)
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

The top-level keys are [`sessions`](ds:block?id=sessionsspec-yzshjgvk), [`logins`](ds:block?id=loginspec-e68nnxxt), `providers`, [`profiles`](ds:block?id=profilespec-jymp7n69) and [`shell`](ds:block?id=shellspec-nagmshhq) ([schema](ds:block?id=file-charr88x)); `sessions.root` defaults to [`~/agx-sessions`](ds:block?id=defaultsessionsdir-xhtaakn6). A login names exactly one of `token: stored`, `token_cmd` or `token_env`, and [`token:` accepts only the word `stored`](ds:block?id=resolvelogin-6dbygptz): a raw token pasted there makes every command refuse the file, without echoing the token. `providers.<id>.login` and a profile's `login:` name a login of the same provider, or `home` (never `auto`, which is only a flag); see [Logins](#logins--use-any-accounts-quota-from-any-home).

Raw login tokens live in `tokens.yaml` beside config.yaml (0600, never synced). Secrets use [`gopass:<path>` (runs `gopass show -o`) or `env:<NAME>`](ds:block?id=parse-39f37ayc). When a provider has any configured profile, [discovery for that provider is switched off](ds:block?id=resolve-3amwuw3d), so the config is the complete list.

## Shell integration

A program cannot change its parent shell's directory, so add this to `~/.zshrc` (or `~/.bashrc`):

```sh
eval "$(agx shell-init zsh)"
```

It defines an `agx` function: for `new`, `resume` and `ws`, the binary writes a short-lived launch plan (never containing secrets), the function `cd`s into the session folder, reports it to your terminal (OSC 7) so new tabs open there, and then starts the agent. It also defines one function per `shell.aliases` entry, so `cl`, `clw`, `clnew` and friends keep working, and `aglogin NAME`, which exports `AGX_LOGIN=NAME` so every run in that terminal spends that login's quota (it checks the name first; `aglogin` with no argument unsets it). Without it, every command still works; your shell just stays where it was.

## JSON output

Every listing command has `--json`, emitting a stable envelope:

```json
{ "schema_version": 1, "kind": "usage.list", "count": 3, "data": [ … ] }
```

[Kinds](ds:block?id=jsonkinds-gqp2ydfe): `usage.list`, [`plan.list`](ds:block?id=kindplanlist-pkqtaqac), `profile.list`, `conversation.list`, `workspace.list`, `session.list`, `session.gc`, `session.promote`, [`session.move`](ds:block?id=kindsessionmove-kyhtpccm), [`login.list`](ds:block?id=kindloginlist-g3cessw2), `doctor.report`, `version.show`. A `login.list` row never carries a token value.

```sh
agx usage --json | jq '.data[] | {profile, windows: [.windows[] | {label, percent}]}'
agx resume --all --list --json | jq -r '.data[] | "\(.updated)  \(.title)"'
```

## How it works

| | Claude Code | Codex |
|---|---|---|
| Accounts | one per config dir: `~/.claude`, or `CLAUDE_CONFIG_DIR` (`~/.claude-work`, …) | one per `CODEX_HOME` (default `~/.codex`) |
| Login read from | `<home>/.credentials.json` and the [macOS Keychain entry `Claude Code-credentials-<sha256(home)[:8]>`](ds:block?id=claudekeychain-58cn2d6s) (freshest wins) | `<home>/auth.json` |
| Usage from | [`GET api.anthropic.com/api/oauth/usage`](ds:block?id=claudeendpoints-hm4t7ueg) (what `/usage` calls) | [`GET chatgpt.com/backend-api/wham/usage`](ds:block?id=defaultendpoint-tvt3cj6w) |
| Conversations | [`<home>/projects/<path with non-alphanumerics as ->/<id>.jsonl`](ds:block?id=encodepath-acf2dnds); titles from `ai-title` / `custom-title` records | [`<home>/sessions/YYYY/MM/DD/rollout-*.jsonl`](ds:block?id=sessionsdir-8yekgwcc) |
| Resume | `claude --resume <id>` in the conversation's folder | `codex resume <id>` |

agx never refreshes a token: refreshing replaces the saved login and can log the agent CLI out. A [login](#logins--use-any-accounts-quota-from-any-home) is a separate, long-lived token you add yourself; agx passes it to the agent in an environment variable and leaves the home's own login untouched. An expired login is reported with the command that makes the CLI refresh it itself. When launching on the default home, agx [*removes* `CLAUDE_CONFIG_DIR` / `CODEX_HOME`](ds:block?id=homeenv-cd4k5f3z) from the environment, because setting Claude's variable even to `~/.claude` switches it to a different Keychain entry and the account looks logged out. Transcripts are never read whole: the folder comes from the start of each file and the title from its end, so `resume` stays fast even with gigabytes of history.

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
| `github.com/khanakia/agx/login` | named logins: resolve a token from a private token file, a command or an env var without ever putting it in an error; the 0600 token file |

A test in `internal/archtest` enforces the layering (the domain model imports only the standard library; libraries never import the CLI framework).

## Development

```sh
task check      # gofmt, go vet, staticcheck, race tests, cross-compile for every platform, build
task test       # tests only
task run -- resume --list
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the dev loop and conventions.

CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs `task check` plus golangci-lint on every push and pull request to `main`. To release, move the `[Unreleased]` notes in CHANGELOG.md under a new `## [X.Y.Z] - date` heading, commit, then `git tag vX.Y.Z && git push --tags`: the release workflow tests, builds every platform with checksums and publishes the GitHub release with that CHANGELOG section as its notes.

## FAQ

**Can I see how many tokens I have left on my Claude or Codex plan?**

No tool can. Subscription plans have no fixed token budget, and the servers only report the percentage of each window used. agx shows that percentage for every account, which is the most precise figure that exists.

**Does checking usage use up my quota?**

No. `agx` / `agx usage` make one read-only GET per account and never call a model. The exception is a token login, which cannot read that endpoint: `agx usage --logins`, `--login NAME`, `--login auto` and `agx login check` send it one minimal model request (a probe), which spends a few tokens of that login's quota. Nothing probes unless you pass one of those.

**Does it send my token or data anywhere?**

Only to the vendor's own API (`api.anthropic.com` for Claude, `chatgpt.com` for Codex), the same place the agent CLI sends it. There is no telemetry.

**Will it log me out of Claude Code or Codex?**

No. It only reads the stored login and never refreshes or rewrites it. An expired login is reported, not refreshed.

**How do I add a second Claude account?**

Start Claude Code with `CLAUDE_CONFIG_DIR=~/.claude-work claude`, log in once, and agx discovers the `~/.claude-work` home as profile `work`.

**How does `-p auto` choose?**

It fetches usage for each plan-billed account of the provider, scores each by its fullest window (because any one window at 100% blocks you), and [picks the lowest score; ties go to config order](ds:block?id=best-3rrnjnxk). API-billed profiles are never picked.

**After `agx sessions promote`, does Claude still have my conversation in the new folder?**

Yes. Promote moves the Claude history along with the folder, so `claude -c`, `claude --resume` and `agx resume` in the new folder continue the same chat (verified against a live Claude Code 2.1.281 session). Codex conversations are not tied to a folder and stay resumable by id.

**I worked in a folder on my personal account — can I switch it to my work account?**

Yes: `agx sessions move --to work` in that folder moves its Claude conversations (with their tool results, checkpoints and project memory) to work, after a backup, so `agx resume` and `claude -c` continue them there. Try it with `--dry-run` first. Codex conversations can't be moved yet.

**When does my Claude subscription renew?**

agx can't tell you: the login it reads doesn't have access to billing details. `agx plan` shows the plan, status and start date; the next charge date is on claude.ai → Settings → Billing.

**Can I run `agx` on every shell prompt or in a tight loop?**

Don't: the vendors rate-limit their usage endpoints, and polling many times a minute gets an account answered with HTTP 429. agx then shows ["usage endpoint rate-limited this account; try again shortly](ds:block?id=errratelimited-d974fprh) [(retry after …)"](ds:block?id=ratelimiterror-69hqnbfz) for that account and keeps showing the others. Every few minutes (for example `watch -n 300 agx`) is fine.

**Is it safe to run `agx sessions gc --yes`?**

It [only removes folders that contain no files, have no conversation history in any provider, and are older than `--min-age`](ds:block?id=plangc-pxge66ab) ([24h by default](ds:block?id=defaultgcminage-67k288nh)), and [deletion uses a call that cannot remove a non-empty directory](ds:block?id=removeempty-tndggau3).

**Does it work with API-key or third-party backends like Kimi?**

Yes, as a `billing: api` profile with its own `env` and `secrets`. Those launch normally and show "billed per token" in usage.

**Does it work on Linux or Windows?**

On Linux, logins are read from the credential files, so it works. The Keychain lookup is macOS-only and is skipped elsewhere. It compiles for Windows, where launching runs the agent as a child process; Windows is otherwise untested.

**Can my work folder spend my personal account's quota?**

Yes. Add the personal account as a login (`claude setup-token` while logged in as personal, then `agx login add personal`) and run `agx run -p work --login personal`, or make it the default with `agx login use personal -p work`. The work home keeps its history; only the quota changes. See [Logins](#logins--use-any-accounts-quota-from-any-home).

**Is it safe to keep a token in `~/.agx/tokens.yaml`?**

It is as safe as an ssh private key: a 0600 file in a 0700 folder that agx refuses to read once anyone else can. Never sync, share or commit it. If you would rather keep tokens in a password manager, add the login with `--cmd "gopass show -o …"` (or `op read …`) and nothing is stored.

**agx says "token file is readable by others".**

tokens.yaml has group or other permission bits, for example after a copy or a restore. Run the `chmod 600 …` the message prints. If the file was readable on a shared machine or synced somewhere, treat the tokens in it as exposed: revoke them and add them again.

**agx says "a raw token does not belong in config.yaml".**

A login's `token:` in config.yaml held something other than `stored`. agx cannot load the config until that line is gone, so delete that login's entry from config.yaml by hand, then add it again with `agx login add NAME --stdin`, which puts the value in tokens.yaml. If config.yaml was synced or committed, revoke that token.

**Does `agx login rm` revoke the token?**

No. It removes the login from agx only. Revoke the token in your claude.ai account settings.

**Can agx pick the login with the most headroom, like `-p auto`?**

Yes, with `--login auto` on `run`, `new`, `resume` or `ws`. It compares the home's own login with every login that fits the profile and runs on the one with the most room, saying which and why. A setup-token cannot read the usage endpoint, so each login is checked with a probe that spends a few tokens of its quota; that is why `auto` is only ever a flag you type, never a default.

**Is this an official Anthropic or OpenAI tool?**

No. It is an independent open-source project, not affiliated with Anthropic or OpenAI. The usage endpoints are undocumented and could change; parsing is defensive and never hides a limit it doesn't recognise.

<!-- docsync: the code blocks this README cites (remote defs, so no directive lives in the Go source). Run `task docs:check`. -->
<!-- ds:def id=newusagecmd-r3ert49b file=internal/cli/usage.go pick=symbol:newUsageCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newplancmd-urn3m4qd file=internal/cli/plan.go pick=symbol:newPlanCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newprofilescmd-ndr78yh5 file=internal/cli/profiles.go pick=symbol:newProfilesCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newruncmd-jgrurvce file=internal/cli/launch.go pick=symbol:newRunCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newnewcmd-u4uqk853 file=internal/cli/launch.go pick=symbol:newNewCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newresumecmd-wp72e3ak file=internal/cli/launch.go pick=symbol:newResumeCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newwscmd-s5rg592z file=internal/cli/browse.go pick=symbol:newWsCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newsessionslscmd-tv2vrc6k file=internal/cli/sessions.go pick=symbol:newSessionsLsCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newsessionsgccmd-qhhnyct9 file=internal/cli/sessions.go pick=symbol:newSessionsGCCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newsessionspromotecmd-tdxnub7p file=internal/cli/sessions.go pick=symbol:newSessionsPromoteCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newsessionsmovecmd-hpm92rna file=internal/cli/move.go pick=symbol:newSessionsMoveCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newdoctorcmd-ndsdf4zg file=internal/cli/doctor.go pick=symbol:newDoctorCmd owner=@khanakia stability=stable -->
<!-- ds:def id=newshellinitcmd-3fw2m4vd file=internal/cli/shellinit.go pick=symbol:newShellInitCmd owner=@khanakia stability=stable -->
<!-- ds:def id=jsonkinds-gqp2ydfe file=internal/cli/usage.go pick=symbol:kindUsageList owner=@khanakia stability=stable -->
<!-- ds:def id=kindplanlist-pkqtaqac file=internal/cli/plan.go pick=symbol:kindPlanList owner=@khanakia stability=stable -->
<!-- ds:def id=kindsessionmove-kyhtpccm file=internal/cli/move.go pick=symbol:kindSessionMove owner=@khanakia stability=stable -->
<!-- ds:def id=exitcodes-35ygnun6 file=internal/cli/app.go pick=symbol:ExitUsage owner=@khanakia stability=stable -->
<!-- ds:def id=defaultusagetimeout-sz3xwdbj file=internal/cli/usage.go pick=symbol:defaultUsageTimeout owner=@khanakia stability=stable -->
<!-- ds:def id=defaultresumelimit-zc5uhvc4 file=internal/cli/launch.go pick=symbol:defaultResumeLimit owner=@khanakia stability=stable -->
<!-- ds:def id=defaultgcminage-67k288nh file=internal/cli/sessions.go pick=symbol:defaultGCMinAge owner=@khanakia stability=stable -->
<!-- ds:def id=app-runmove-gs5yg4x2 file=internal/cli/move.go pick=symbol:app.runMove owner=@khanakia stability=stable -->
<!-- ds:def id=file-charr88x file=config/config.go pick=symbol:File owner=@khanakia stability=stable -->
<!-- ds:def id=sessionsspec-yzshjgvk file=config/config.go pick=symbol:SessionsSpec owner=@khanakia stability=stable -->
<!-- ds:def id=profilespec-jymp7n69 file=config/config.go pick=symbol:ProfileSpec owner=@khanakia stability=stable -->
<!-- ds:def id=shellspec-nagmshhq file=config/config.go pick=symbol:ShellSpec owner=@khanakia stability=stable -->
<!-- ds:def id=decode-qymvq95s file=config/config.go pick=symbol:Decode owner=@khanakia stability=stable -->
<!-- ds:def id=resolve-3amwuw3d file=config/config.go pick=symbol:Resolve owner=@khanakia stability=stable -->
<!-- ds:def id=defaultsessionsdir-xhtaakn6 file=config/config.go pick=symbol:defaultSessionsDirName owner=@khanakia stability=stable -->
<!-- ds:def id=appidentity-65c6dy83 file=internal/appmeta/appmeta.go pick=symbol:EnvPrefix owner=@khanakia stability=stable -->
<!-- ds:def id=parse-39f37ayc file=secret/secret.go pick=symbol:Parse owner=@khanakia stability=stable -->
<!-- ds:def id=encodepath-acf2dnds file=provider/claude/conversations.go pick=symbol:EncodePath owner=@khanakia stability=stable -->
<!-- ds:def id=claudekeychain-58cn2d6s file=provider/claude/constants.go pick=symbol:keychainService owner=@khanakia stability=stable -->
<!-- ds:def id=claudeendpoints-hm4t7ueg file=provider/claude/constants.go pick=symbol:DefaultEndpoint owner=@khanakia stability=stable -->
<!-- ds:def id=defaultendpoint-tvt3cj6w file=provider/codex/codex.go pick=symbol:DefaultEndpoint owner=@khanakia stability=stable -->
<!-- ds:def id=sessionsdir-8yekgwcc file=provider/codex/codex.go pick=symbol:sessionsDir owner=@khanakia stability=stable -->
<!-- ds:def id=homeenv-cd4k5f3z file=provider/env.go pick=symbol:HomeEnv owner=@khanakia stability=stable -->
<!-- ds:def id=ratelimiterror-69hqnbfz file=provider/ratelimit.go pick=symbol:RateLimitError owner=@khanakia stability=stable -->
<!-- ds:def id=errratelimited-d974fprh file=provider/provider.go pick=symbol:ErrRateLimited owner=@khanakia stability=stable -->
<!-- ds:def id=applymoveplan-j9u9m6j7 file=provider/move.go pick=symbol:ApplyMovePlan owner=@khanakia stability=stable -->
<!-- ds:def id=best-3rrnjnxk file=pick/pick.go pick=symbol:Best owner=@khanakia stability=stable -->
<!-- ds:def id=plangc-pxge66ab file=sessions/sessions.go pick=symbol:PlanGC owner=@khanakia stability=stable -->
<!-- ds:def id=removeempty-tndggau3 file=sessions/sessions.go pick=symbol:RemoveEmpty owner=@khanakia stability=stable -->
<!-- ds:def id=promote-bdf6x7wh file=sessions/sessions.go pick=symbol:Promote owner=@khanakia stability=stable -->

<!-- ds:def id=newlogincmd-zhxdkaat file=internal/cli/login.go pick=symbol:newLoginCmd owner=@khanakia stability=stable -->
<!-- ds:def id=app-picklogin-p3tyusqh file=internal/cli/login.go pick=symbol:app.pickLogin owner=@khanakia stability=stable -->
<!-- ds:def id=kindloginlist-g3cessw2 file=internal/cli/login.go pick=symbol:kindLoginList owner=@khanakia stability=stable -->
<!-- ds:def id=expirywarnwindow-fnenahy6 file=internal/cli/login.go pick=symbol:expiryWarnWindow owner=@khanakia stability=stable -->
<!-- ds:def id=revokehint-ngx5vax3 file=internal/cli/login.go pick=symbol:revokeHint owner=@khanakia stability=stable -->
<!-- ds:def id=loginspec-e68nnxxt file=config/config.go pick=symbol:LoginSpec owner=@khanakia stability=stable -->
<!-- ds:def id=resolvelogin-6dbygptz file=config/logins.go pick=symbol:resolveLogin owner=@khanakia stability=stable -->
<!-- ds:def id=tokenfile-checkperm-4epx2b72 file=login/tokens.go pick=symbol:TokenFile.CheckPerm owner=@khanakia stability=stable -->
<!-- ds:def id=cmdtimeout-44pfsqzk file=login/login.go pick=symbol:cmdTimeout owner=@khanakia stability=stable -->
<!-- ds:def id=provider-loginenv-n8x9u9rf file=provider/claude/login.go pick=symbol:Provider.LoginEnv owner=@khanakia stability=stable -->
<!-- ds:def id=setuptokenlifetime-nebd9cbt file=provider/claude/login.go pick=symbol:SetupTokenLifetime owner=@khanakia stability=stable -->
<!-- ds:def id=probeclient-probe-i9609s2l file=provider/claude/probe.go pick=symbol:ProbeClient.Probe owner=@khanakia stability=stable -->
<!-- ds:def id=defaultprobemodel-g7o7rdkd file=provider/claude/probe.go pick=symbol:DefaultProbeModel owner=@khanakia stability=stable -->
<!-- ds:def id=app-autologin-a4w0xz3h file=internal/cli/loginusage.go pick=symbol:app.autoLogin owner=@khanakia stability=stable -->
<!-- ds:def id=app-resolveloginvalue-10wd6ob4 file=internal/cli/login.go pick=symbol:app.resolveLoginValue owner=@khanakia stability=stable -->

## License

[Apache-2.0](LICENSE) © 2026 khanakia

<sub>agx: open-source CLI toolkit for AI coding agents — Claude Code and OpenAI Codex usage limits across multiple accounts, auto account selection, cross-account conversation resume, session folder management. Local, read-only, no telemetry, written in Go.</sub>
