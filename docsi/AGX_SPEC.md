---
title: agx — spec
---

# agx — one CLI for your AI coding agents and all their accounts

<DocStatus state="approved" owner="khanakia" updated="2026-09-24"></DocStatus>

<Callout type="info" title="Implementation status">

Shipped 2026-09-24: every command in §4 is built, tested and verified against the real machine (two Claude accounts + one Codex login). §16 records the file map and every delta from the original design; keep it current with the code.

</Callout>

## 1. Problem

Daily agent work on this machine runs through a pile of zsh functions in `~/.zsh_luci`: `cl` / `clw` (launch Claude on the personal / work account), `clnew` / `clwnew` / `clnews` / `clwnews` (make a timestamped session folder and launch), `clwho` (which account am I on), `claude-kimi` / `opencode-kimi` (one-off sessions on another backend). They work, but:

- **The account is picked by hand.** Personal can sit at 77% weekly while work is at 2%, and `clnew` still launches personal.
- **Resuming is manual and error-prone.** Claude Code keeps conversation history *per config dir*, so resuming a work session with the personal launcher finds nothing. Nothing records which account a session folder used.
- **Session folders pile up.** 40 of 58 folders under `sessions/` are empty.
- **`clwho` is wrong for personal** — it checks the stale legacy keychain entry, not the live `.credentials.json`.
- **Adding an account or a provider means writing more shell functions**, and flags must be kept in sync by hand across four near-identical launchers.
- **Usage is invisible** without opening claude.ai or `/usage` per account (fixed by the first agx feature, `agx usage`).

## 2. Goals and non-goals

**Goals**

- One binary, `agx`, that is **provider-neutral**: Claude first, Codex second, more later behind one interface.
- Show plan usage for every account of every provider at once.
- Launch, create and resume sessions on the right account — automatically picking the account with the most headroom when asked.
- Manage the sessions folder: list, garbage-collect empty folders safely, promote a session into a real project without orphaning its history.
- Replace every `cl*` shell function with a thin, generated shell layer so muscle memory survives.
- Built on [voltkit](https://github.com/khanakia/voltkit): `appdir` for where config lives, `output` for the `--json` envelope, `versioncmd` for `agx version`, cobra for commands, `volt` for CI and releases.

**Non-goals**

- Never refresh, write, copy or print an OAuth token. agx only reads logins the vendor CLI stored.
- No daemon, no background polling, no telemetry, no cache files. Every command computes from source.
- Not a replacement for the vendor CLIs — agx builds the right command line and `exec`s them.
- No GUI / menu-bar app.

## 3. Concepts

| Term | Meaning |
|---|---|
| **Provider** | A vendor CLI agx knows how to drive: `claude`, `codex`. Implements the `provider.Provider` interface (§8). |
| **Home** | The provider's config directory: `~/.claude`, `~/.claude-work`, `~/.codex`. One home = one login = one history store. |
| **Profile** | A named way to launch a provider: provider + home + args + env + secrets + billing. Several profiles can share a home (e.g. `personal` and `kimi` both use `~/.claude`). |
| **Billing** | `plan` (subscription with usage windows — the default) or `api` (billed per token; no usage windows, e.g. Kimi via `ANTHROPIC_BASE_URL`). |
| **Window** | One usage limit: a percent used plus a reset time, e.g. Claude's 5-hour session, weekly all-models, weekly Fable; Codex's 5-hour, weekly or 30-day window. |
| **Session folder** | A directory under `sessions.root` created by `agx new`, named `[slug_]YYYYMMDD_HHMMSS`. |
| **Conversation** | One resumable transcript stored by the provider (a Claude `.jsonl` under `<home>/projects/`, a Codex rollout under `<home>/sessions/`). |

## 4. Command reference

| Command | Does | Replaces |
|---|---|---|
| `agx` / `agx usage [profile…]` | Plan usage for every plan-billed profile (deduplicated by home), bars + reset countdown; `--json`, `--color`, `--timeout`, `--provider` | — |
| `agx profiles` | Every profile: provider, home, account email, plan, login state and expiry, billing, default | `clwho` |
| `agx run [-p profile\|auto] [-- args…]` | Exec the provider CLI in the current directory with the profile's args / env / secrets | `cl`, `clw`, `claude-kimi` |
| `agx new [-p profile\|auto] [slug…]` | Create `sessions.root/[slug_]YYYYMMDD_HHMMSS`, enter it, launch | `clnew`, `clwnew`, `clnews`, `clwnews` |
| `agx resume [query]` | Pick a conversation (this folder first, else recent everywhere) and continue it **on the profile whose home holds it**; `--list`, `--all`, `--json`, `--last` | — |
| `agx sessions ls` | Session folders: name, age, file count, conversations, last activity, which homes hold history | — |
| `agx sessions gc` | Plan (default) or delete (`--yes`) empty session folders with no conversations anywhere, older than `--min-age` | — |
| `agx sessions promote <folder> <name>` | Move a session folder into `sessions.promote_root/<name>` and move its Claude history with it; `--dry-run`, `--git-init` | manual `mv` |
| `agx doctor` | Config source, sessions root (and whether it sits inside a git repo), every profile's binary / home / login / secret backend, shell integration | — |
| `agx shell-init zsh` | Print the shell layer (§10): the `agx` wrapper that can `cd`, plus configured aliases | the `cl*` functions |
| `agx version` | voltkit `versioncmd`: version, commit, build time, source | `--version` |

Global conventions (from voltkit): stdout is data, stderr is diagnostics; every command with output has a local `--json` that emits the voltkit `output` envelope; exit codes `0` ok, `1` runtime failure (including "some accounts failed"), `2` usage error; every command sets `Args` so a typo never runs something else.

## 5. Configuration

### 5.1 Where

Resolved with voltkit `appdir.New("agx", appdir.WithLayout(appdir.AllHomeDotfile))`, so the file is `~/.agx/config.yaml`, overridable with `AGX_CONFIG_DIR` or `AGX_HOME` (appdir's env rungs). `agx doctor` prints the path and which rung decided it.

**The file is optional.** With no config, agx discovers profiles (§5.3) and uses built-in defaults, so the tool works on a fresh machine.

### 5.2 Schema

```yaml
# ~/.agx/config.yaml
sessions:
  root: ~/work/sessions      # where `agx new` creates folders
  promote_root: ~/work       # default target for `sessions promote`

providers:                     # per-provider defaults, merged under every profile of that provider
  claude:
    args: [--dangerously-skip-permissions, --chrome, --remote-control]

profiles:                      # order matters: it breaks `auto` ties and orders output
  - name: personal
    provider: claude
    home: ~/.claude
    default: true              # used when -p is omitted
  - name: work
    provider: claude
    home: ~/.claude-work
  - name: kimi
    provider: claude
    home: ~/.claude            # shares personal's home (and therefore its history)
    billing: api               # no usage windows; excluded from `auto`
    args_replace: true         # use ONLY these args, not providers.claude.args
    args: [--dangerously-skip-permissions, --allow-dangerously-skip-permissions]
    env:
      ANTHROPIC_BASE_URL: https://api.moonshot.ai/anthropic
      ANTHROPIC_MODEL: "kimi-k3[1m]"
    secrets:                   # resolved at launch, never written anywhere
      ANTHROPIC_AUTH_TOKEN: gopass:personal/ai/moonshot-kimi-x
  - name: codex
    provider: codex
    home: ~/.codex

shell:
  aliases:                     # emitted by `agx shell-init zsh`
    cl: run -p personal
    clw: run -p work
    clnew: new -p personal
    clwnew: new -p work
    cla: run -p auto
    clanew: new -p auto
```

Rules:

- Unknown keys are an error (`yaml.v3` `KnownFields(true)`), matching `.volt.yml`.
- `~` in paths is expanded; paths are cleaned and made absolute.
- `name` is unique, `[a-z0-9-]+`, and `auto` is reserved.
- `secrets` values are `scheme:ref`. Schemes: `gopass:` (runs `gopass show -o <ref>`) and `env:` (reads another environment variable). Unknown schemes are a config error.
- Profile `args` append to `providers.<id>.args` unless `args_replace: true`.

### 5.3 Discovery (no config, or a provider with no configured profiles)

- **Claude:** `~/.claude` becomes profile `personal` (default); each `~/.claude-<x>` directory that contains a marker (`settings.json`, `projects/`, `.credentials.json`, `.claude.json`) becomes profile `<x>`. `~/.claude-worktrees` has no marker and is skipped.
- **Codex:** `~/.codex` with an `auth.json` becomes profile `codex`.
- Discovered profiles are marked `source: discovered` in `agx profiles` so it is always clear whether a profile came from the file.

## 6. Provider details

### 6.1 Claude

| Concern | How |
|---|---|
| Login | `<home>/.credentials.json` and keychain `Claude Code-credentials-<sha256(home)[:8]>` (plus bare `Claude Code-credentials` for the default home); freshest `expiresAt` wins. Never refreshed. |
| Identity | `<home>/.claude.json` → `oauthAccount.emailAddress`, `organizationName`; for the default home, `~/.claude.json`. Plan from the credential's `subscriptionType` + `rateLimitTier` (`Max (20x)`). |
| Usage | `GET https://api.anthropic.com/api/oauth/usage`, `Authorization: Bearer`, `anthropic-beta: oauth-2025-04-20`. Read `limits[]` (kind, group, percent, severity, resets_at, scope, is_active); fall back to `five_hour` / `seven_day[_opus\|_sonnet]` when absent. |
| Launch | `claude <args…>` with `CLAUDE_CONFIG_DIR=<home>` **only when home is not `~/.claude`**. For the default home the variable is actively *removed* from the environment: setting it, even to `~/.claude`, switches Claude Code to the hashed keychain name and the account appears logged out. |
| Conversations | `<home>/projects/<encode(dir)>/<sessionId>.jsonl`, where `encode` replaces every non-`[A-Za-z0-9]` byte with `-` (verified: 158/158 existing project dirs). Title = last `custom-title`, else last `ai-title`, read from the file's tail (last 256 KiB); cwd from the first line carrying `cwd` in the head. Files are never read whole — `~/.claude/projects` is ~2 GB. |
| Resume | `claude <args…> --resume <sessionId>` run in the conversation's cwd. |
| History move | `promote` renames `<home>/projects/<encode(old)>` to `<home>/projects/<encode(new)>` for every Claude home that has one; refuses if the target exists. |

### 6.2 Codex

| Concern | How |
|---|---|
| Login | `<home>/auth.json` → `tokens.access_token`, `tokens.account_id` (`auth_mode: chatgpt`). An `OPENAI_API_KEY`-only file means billing `api`. Never refreshed. |
| Usage | `GET https://chatgpt.com/backend-api/wham/usage` with `Authorization: Bearer` and `ChatGPT-Account-Id`. Read `email`, `plan_type`, `rate_limit.{primary,secondary}_window.{used_percent, reset_at (unix s), limit_window_seconds}`, `rate_limit.limit_reached`. Windows are labelled by length: 18000 s → "5-hour", 604800 s → "Weekly", 2592000 s → "30-day", else "N-day". |
| Launch | `codex <args…>` with `CODEX_HOME=<home>` only when home is not `~/.codex`. |
| Conversations | `<home>/sessions/YYYY/MM/DD/rollout-*.jsonl`; the first line is `session_meta` with `payload.id` and `payload.cwd`. |
| Resume | `codex resume <id>` in the conversation's cwd. |
| History move | Not supported (Codex keys conversations by id, not path); `promote` warns and leaves Codex history in place. |

### 6.3 API-billed profiles (e.g. Kimi)

`billing: api` profiles launch like any other but have no usage windows: `agx usage` lists them as `billed per token`, and `auto` never picks them. Secrets come from `gopass` at exec time and exist only in the child's environment.

## 7. Auto-pick policy (`-p auto`)

Candidates: plan-billed profiles of the requested provider (default `claude`) whose usage fetch succeeded. Score = the **highest** percent across all of that profile's windows, because any one window at 100% blocks work. Pick the lowest score; ties go to config order. Print the decision on stderr, e.g. `agx: auto → work (max 5%) over personal (max 77%)`. If no candidate qualifies, fail with every profile's reason. Profiles sharing a home are scored once.

## 8. Architecture

### 8.1 Layout and layering

```filetree
agx/
  main.go                    thin: signal context, root.ExecuteContext, exit code
  provider/                  domain model + Provider interface — stdlib only, zero agx imports
  provider/claude/           Claude implementation (login, usage, launch, conversations, history move)
  provider/codex/            Codex implementation
  config/                    YAML load, validation, discovery merge → []provider.Profile
  secret/                    scheme:ref resolution (gopass, env) behind an interface
  pick/                      auto-pick policy — pure
  sessions/                  session folders: naming, listing, gc plan, promote plan — fs only
  internal/appmeta/          agx identity (name, env prefix, dir name)
  internal/render/           terminal rendering (bars, tables) — no I/O besides io.Writer
  internal/shellinit/        zsh script generation
  internal/cli/              cobra commands — the ONLY package importing cobra and voltkit command modules
  docsi/                     this spec + in-repo memory
```

Import rules, enforced by `internal/archtest` (a test that parses imports with `go/parser`, so multi-line import blocks are seen):

1. `provider` imports only the standard library.
2. `provider/*`, `config`, `secret`, `pick`, `sessions` never import `internal/...` or cobra.
3. Only `internal/cli` imports `github.com/spf13/cobra` or `github.com/khanakia/voltkit/versioncmd`.
4. `main` imports only `internal/cli` and `internal/appmeta` (plus stdlib).

`provider/...`, `config`, `secret`, `pick` and `sessions` are public packages so another tool can import, for example, `github.com/khanakia/agx/provider/claude` to read Claude usage.

### 8.2 The provider interface

```go
package provider

type ID string // "claude", "codex"

type Provider interface {
    ID() ID
    Binary() string                                          // "claude", "codex"
    DefaultHome(userHome string) string                      // ~/.claude, ~/.codex
    Discover(userHome string) ([]Profile, error)             // implicit profiles (§5.3)
    Identity(p Profile) (Identity, error)                    // email / org / plan / login expiry — local only
    Usage(ctx context.Context, p Profile) (Usage, error)     // ErrAPIBilled for billing=api
    Launch(p Profile, req LaunchRequest) (Command, error)    // argv + env + dir, not executed
}

// Optional upgrades — a provider lacking one is never forced to fake it.
type ConversationLister interface {
    Conversations(ctx context.Context, home string, q ConversationQuery) ([]Conversation, error)
}
type HistoryMover interface {
    MoveHistory(home, fromDir, toDir string) (moved bool, err error)
}
type HistoryChecker interface {                               // cheap "any conversation for dir?" for gc / ls / promote
    HasHistory(home, dir string) bool
}
```

`Command` is plain data (`Path`, `Args`, `Env`, `Dir`); `internal/cli` executes it with `syscall.Exec` so the vendor CLI owns the terminal directly.

## 9. Sessions

- **Naming:** `[slug_]YYYYMMDD_HHMMSS`, where slug is the args lowercased with runs of non-`[a-z0-9]` collapsed to `_` (identical to today's `_clnew`). Local time. If the name exists (two `new`s in one second) a `_2`, `_3` suffix is added rather than reusing a folder.
- **`sessions.root` must not be inside a git repo** (Claude Code keys memory to the enclosing git root, so a repo would pool every session's memory); `doctor` warns if it is.
- **gc** deletes a folder only when all hold: it contains no files (`.DS_Store` ignored); no provider home has a conversation for it (Claude by encoded path, Codex by cwd index); it is older than `--min-age` (default 24h). Deletion uses `os.Remove` on each empty directory, which cannot delete content by construction. Dry run is the default.
- **promote** moves the folder with `os.Rename` (same volume) and refuses to overwrite. Claude history for the folder moves with it (§6.1); Codex history is left and reported. `--dry-run` prints the plan; `--git-init` runs `git init` in the new location.

## 10. Shell integration

A binary cannot change its parent shell's directory, so `new` and `resume` use a **plan-file protocol** when wrapped:

1. `eval "$(agx shell-init zsh)"` defines an `agx` shell function and the configured aliases.
2. For `new` / `resume`, the function creates a private temp file and runs `AGX_PLAN_FILE=<file> command agx new …`.
3. With `AGX_PLAN_FILE` set, the binary does the interactive part (create folder / pick conversation), writes `{dir, profile, resume_id, provider, args}` as JSON (mode 0600) and exits 0 without launching.
4. The function `cd`s into `dir`, emits OSC 7 so new terminal tabs inherit it, and runs `command agx exec --plan <file>`, which deletes the file, resolves the profile and secrets fresh, and `exec`s the provider.

Secrets are never written to the plan file. Without the wrapper, `new` / `resume` still work — they `chdir` and `exec` directly — only the parent shell's directory stays where it was.

## 11. Output kinds (`--json`)

| Command | `kind` |
|---|---|
| `usage` | `usage.list` |
| `profiles` | `profile.list` |
| `resume --list` | `conversation.list` |
| `sessions ls` | `session.list` |
| `sessions gc` | `session.gc` |
| `sessions promote` | `session.promote` |
| `doctor` | `doctor.report` |
| `version` | `version.show` (versioncmd) |

## 12. Safety invariants

1. Tokens are read, never refreshed, written, copied, logged or placed in a plan file.
2. `CLAUDE_CONFIG_DIR` / `CODEX_HOME` are set only for non-default homes and removed for the default home.
3. gc can only remove empty directories; promote never overwrites.
4. Secrets resolve at exec time into the child environment only.
5. Every network call is a read-only GET to the vendor's own API; nothing else leaves the machine.

## 13. Migration from `~/.zsh_luci`

| Today | agx |
|---|---|
| `cl` / `clw` | `agx run -p personal` / `-p work` (aliases `cl` / `clw` via `shell.aliases`) |
| `clnew` / `clwnew` / `clnews x` / `clwnews x` | `agx new [-p work] [x]` |
| `clwho [work]` | `agx profiles` |
| `claude-kimi` | profile `kimi` + `agx run -p kimi` |
| `CL_FLAGS` | `providers.claude.args` |
| `CLNEW_HOME` | `sessions.root` |
| `_emit_cwd` | built into `shell-init` |

Switching is one line in `.zsh_luci` (`eval "$(agx shell-init zsh)"`) plus deleting the old functions — done by the user, not by agx.

## 14. Decisions

<ADR status="accepted" id="AGX-001" date="2026-09-24" title="Build on voltkit + cobra">

### Context

The user maintains voltkit as the standard kit for Go CLIs; `agx` will grow many subcommands.

### Decision

Use cobra for the command tree, voltkit `appdir` (config location), `output` (JSON envelope) and `versioncmd` (`agx version`), and `volt` for CI/release.

### Consequences

Consistent `--json` and version provenance with every other voltkit CLI. agx becomes the first external consumer of `appdir`/`output`/`versioncmd`, which surfaces any packaging bugs in them.

</ADR>

<ADR status="accepted" id="AGX-002" date="2026-09-24" title="Read-only credentials">

### Context

Refreshing an OAuth token rotates the refresh token; the vendor CLI's stored copy then becomes invalid and the user is logged out.

### Decision

agx never refreshes. An expired login is reported with the exact command that makes the vendor CLI refresh it.

### Consequences

A long-idle account shows "expired" until used once; no risk of breaking live sessions.

</ADR>

<ADR status="accepted" id="AGX-003" date="2026-09-24" title="Plan-file shell protocol">

### Context

`new` / `resume` must change the shell's directory, which a child process cannot do.

### Decision

Two-phase: the binary writes a plan (no secrets), the generated shell function `cd`s and then calls `agx exec --plan`.

### Consequences

All logic stays in tested Go; the shell layer is ~20 generated lines. Works unwrapped too, minus the parent-shell `cd`.

</ADR>

<ADR status="accepted" id="AGX-004" date="2026-09-24" title="YAML config at ~/.agx/config.yaml, optional">

### Context

voltkit ships no config loader; its docs mention both YAML and JSON. The file is hand-edited and holds lists of profiles.

### Decision

YAML (`gopkg.in/yaml.v3`, `KnownFields(true)`), located by `appdir` with the home-dotfile layout. Zero-config discovery covers the common case.

### Consequences

One extra dependency; strict parsing catches typos the way `.volt.yml` does.

</ADR>

## 15. Deferred

- Providers beyond Claude and Codex (OpenCode, Gemini CLI) — the interface is ready; add when used.
- Kimi / Moonshot balance lookup for `billing: api` profiles.
- Homebrew tap and `volt release` — no release yet by the user's choice.
- Notifications when a window crosses a threshold (would need a scheduler — out of scope for a no-daemon tool).

## 16. Implementation status

| Area | Status | Where | Delta vs spec |
|---|---|---|---|
| Domain model + env helpers | shipped | `provider/provider.go`, `provider/env.go` | Added `HistoryChecker` upgrade and shared window-group constants (`GroupSession/Weekly/Plan/Extra`). |
| Claude provider | shipped | `provider/claude/` | Moved from the old `internal/account` + `internal/usage` (history kept via `git mv`). Extra-usage becomes a synthetic "Extra usage" window when enabled. |
| Codex provider | shipped | `provider/codex/` | Email / plan are only known after a usage fetch (Codex stores none locally), so `profiles` shows them blank. |
| Config | shipped | `config/` | Alias commands are restricted to `[A-Za-z0-9 _./:=@+-]` because they are pasted into shell code. A discovered profile whose name collides with a configured one is renamed `<provider>-<name>`. |
| Secrets | shipped | `secret/` | — |
| Auto-pick | shipped | `pick/` | — |
| Session folders | shipped | `sessions/` | Same-second collisions get `_2`, `_3` suffixes. |
| CLI | shipped | `internal/cli/` | Default provider for `run` / `new -p auto` is chosen with `--provider` (default `claude`). `resume` searches everything when a query is given, then applies `--limit`. Windows runs the agent as a child (`exec_other.go`) instead of `exec(2)`. |
| Shell layer | shipped | `internal/shellinit/` | Also supports bash; verified end-to-end in real zsh and bash with a fake agent binary. |
| Layering guard | shipped | `internal/archtest/` | Verified by adding a forbidden import inside a multi-line block: 3 violations reported. |
| Build / release | shipped | `Taskfile.yml`, `.volt.yml` | `task check` adds a cross-compile matrix (verified by removing the Windows shim). Minimum Go 1.26 (voltkit). agx requires `voltkit/output` directly, which is what makes `versioncmd@v0.1.0` resolve — its published go.mod requires a non-existent `output v0.0.0`; fix upstream with a `versioncmd/v0.1.1`. |
| Rate limiting | shipped | `provider/ratelimit.go` | Added after live use: HTTP 429 maps to `provider.ErrRateLimited` with the server's Retry-After, and is never shown as a login problem. |
| Resume after promote | shipped | `provider/claude/conversations.go` | A folder-scoped lookup now trusts the history folder (as Claude Code does) and reports the queried folder; the transcript's recorded cwd is stale after promote. Verified live: `claude -c` in a promoted folder recalled a word from before the move. |
| Promote across disks | shipped | `sessions/crossdevice_*.go` | Rename across filesystems is refused with `ErrCrossDevice` (no copy fallback, by design: atomic or nothing). |
| Test coverage | shipped | `*_test.go`, `main_test.go` | 88.7% of statements across packages; adapters (keychain, gopass, fzf, exec) tested against fake programs; the real binary is built and run in `TestBinary`. |
| Not built | deferred | — | `doctor` does not report the stale legacy `Claude Code-credentials` keychain entry (credential resolution already ignores it by picking the freshest login). Kimi balance, other providers, release: see §15. |
