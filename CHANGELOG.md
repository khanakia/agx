# Changelog

All notable changes to **agx** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.3] - 2026-10-16

### Added

- Logins: run any home on any account's quota. A login is a named, long-lived Claude token (`claude setup-token`) or API key that agx passes to Claude Code at launch, so a home keeps its history, settings and `resume` while the run spends another account's plan. `agx login add NAME` takes the token at a hidden prompt, from `--stdin`, after `--setup` (runs `claude setup-token` first), or as a source read at each launch: `--cmd "<command>"` or `--env VAR`. `--kind api-key` adds an API key instead; `--account` is a label; `--replace` rotates a token and keeps the kind and account.
- `agx login ls` (`--json`, kind `login.list`; never a value), `agx login check [NAME]` (resolves the token and probes its usage; `--no-probe` only resolves), `agx login use NAME|home -p PROFILE` / `--provider ID` (edits config.yaml in place, keeping comments and order, and prints the key before and after; a discovered profile is first written into the file together with its provider's other discovered profiles) and `agx login rm NAME` (refused while a profile or provider default uses it; `--force` clears those references; prints that removing does not revoke the token).
- `--login NAME|home|auto` on `run`, `new`, `resume` and `ws`, carried through the shell layer's plan file. Precedence: `--login`, then `AGX_LOGIN`, then the profile's `login:`, then `providers.<id>.login`, then the home's own login. A login that does not fit the profile (another provider, `billing: api`, or a profile with its own credential in `env:` / `secrets:`) is an error when asked for by name and skipped with a note when it comes from `AGX_LOGIN`.
- `aglogin NAME` in `agx shell-init` (zsh and bash): exports `AGX_LOGIN` for this terminal after checking the name; `aglogin` with no argument unsets it.
- Config keys `logins:` (`name`, `provider`, `kind`, exactly one of `token: stored` / `token_cmd` / `token_env`, `account`, `added`), a profile's `login:` and `providers.<id>.login`. A raw token pasted into config.yaml is rejected without echoing it.
- `~/.agx/tokens.yaml` for pasted tokens: mode 0600 in a 0700 folder, atomic writes, refused when group or others can access it (with the `chmod 600` that fixes it).
- At launch an `oauth-token` login sets `CLAUDE_CODE_OAUTH_TOKEN` and removes `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN`; an `api-key` login sets `ANTHROPIC_API_KEY` and removes the other two. The launch line says `agx: <profile> · login <name>`, and `--dry-run` shows `login:` and `NAME=<token:source>`. A missing, empty or multi-line token, a failing or hung `--cmd` (10 s limit) or an unset `--env` stops the launch; agx never falls back to the home login silently.
- Usage of a login by probe: a setup-token cannot read the usage endpoint (403), so agx sends one minimal Messages request (smallest model, one output token) and reads the `anthropic-ratelimit-unified-5h-*` / `-7d-*` response headers into the same 5-hour and weekly windows. A probe spends a few tokens of the login's plan quota and counts toward its rate limits, so it runs only on request: `agx usage --logins` (one row per login after the homes; JSON rows carry `login`, `account` and `"source": "probe"`, kind unchanged), `agx usage --login NAME`, `agx login check` and `--login auto`. A plain `agx` / `agx usage` never probes. Missing or malformed headers are an error, never a 0% row; api-key logins are never probed.
- `--login auto`: compares the home's own login with every login that fits the profile, using the `-p auto` rule (lowest fullest window; ties go to the home, then config order), skips a login whose token does not resolve or whose probe fails, and says which won and why (`agx: login auto → personal (max 12%) over home (max 64%)`). `auto` is refused in config.yaml, `AGX_LOGIN` and `agx login use`, so probing never becomes a default.
- Expiry: `agx login ls` shows EXPIRES (setup-tokens are taken to last 365 days from `added`; `~` for `--cmd` / `--env` logins) and `agx doctor` warns 30 days ahead. Past it a stored login is refused with the renew command (doctor fails); a `--cmd` / `--env` login warns and runs (doctor warns), because the token behind it may have been rotated since the login was added.
- `agx doctor` checks tokens.yaml (mode, folder mode, inside a git work tree), each stored login's entry, each `--env` source, and expiry.
- Library: package `login` (`Login`, `Resolver`, `TokenFile`, `Clean`, `Compatible`, `ExpiresAt`); `provider.LoginLauncher`, `provider.LoginLifetimer`, `provider.LoginUsageReader`, `provider.ApplyLoginEnv`; `config.Editor` for comment-preserving config edits.

### Changed

- `agx profiles`: the column that showed the home's login state is now `STATE`; `LOGIN` shows the login a run spends and why (`personal (profile)`, `work (AGX_LOGIN)`, `home`). In `--json`, `login` is unchanged and `login_name` / `login_source` are new.
- `--dry-run` always prints a `login:` line.

### Not yet

- `agx plan --login NAME`: the plan comes from the profile endpoint, which refuses setup-tokens, so `agx plan` accepts only `--login home`.
- Codex logins: a Codex plan login exists only as refreshable tokens in `auth.json`, and the API-key variable interactive Codex honours is unverified.

## [0.3.2] - 2026-10-14

### Changed

- `agx resume` no longer falls back to every folder when the current folder has no conversations. It prints `agx: no conversations in <folder>` with the hint `(agx resume --all to pick from every folder)` and exits 1; `--last` and `--list` do the same. `--list --json` prints an empty `conversation.list` envelope and exits 0. Every folder is still one flag away with `--all`.

### Fixed

- Sessions launched by agx (`run`, `new`, `resume`, aliases) no longer inherit `CLAUDE_CODE_CHILD_SESSION`. A terminal opened from inside a Claude Code session carries that marker, and a top-level session started with it does not save its transcript, so it could not be resumed later. agx now removes the variable from the child environment.

## [0.3.1] - 2026-10-14

### Added

- `agx ws` (also `agx resume --browse` / `-b`): a two-level picker. First every folder that has conversations, across all accounts and providers, newest activity first (chat count and profiles per folder); then that folder's chats; the chosen chat resumes on its own account. Esc on the chats goes back to the folders. `--list` prints the folders, `--list --json` as kind `workspace.list`. The shell layer routes `ws` through the plan file, so the shell is left in the chat's folder.
- `agx sessions move -i` / `--pick`: multi-select this folder's chats (fzf `--multi`; Tab marks), then pick the target profile when `--to` is not given (same-provider profiles; the ones holding the chats now are marked). The chosen chats become `--only`; `--dry-run` and every safety check apply. `--all` picks the folder from every workspace first; in a folder with no chats it stops and says to use `--all`. Without fzf, numbers such as `1 3` or `1,3` are accepted.
- `agx resume --list` and the resume picker show the profile each conversation resumes on (the same `config.ForHome` rule the launch uses) in a column after the provider. `--json` (kind `conversation.list`) adds `profile` and `profiles` (every profile on that home, so a shared home such as personal + kimi on `~/.claude` lists both); all existing fields are unchanged. Library: `config.Config.ProfilesForHome`.

### Changed

- `agx resume --list` rows fit on one line: the id is cut to 8 characters (still usable as a query), long titles end in `…`, and a folder that does not fit shows its last path segment. `--json` keeps full values.
- Folders that no longer exist are left out of the workspace lists (`ws`, `sessions move -i --all`).
- voltkit modules now come from `github.com/ubgo/voltkit` (the repo moved from the khanakia account): `appdir` and `output` v0.2.0, `versioncmd` v0.2.1. versioncmd v0.2.1 requires a published `output`, so agx no longer depends on its own direct `output` requirement to build.

### Fixed

- A keychain lookup that timed out could still block until the `security` process's children exited, because they held its output pipe open; the lookup now gives up shortly after the timeout (`exec.Cmd.WaitDelay`).

## [0.3.0] - 2026-09-26

### Added

- `agx plan [profile…]` (alias `subscription`): plan, subscription status and start date for every account, `--json` (kind `plan.list`). Claude reads `/api/oauth/profile`; Codex reports its plan from the usage response. The next renewal date is not shown — it is not available to the Claude Code login (claude.ai → Settings → Billing has it).
- Provider interface `SubscriptionReader`.

### Changed

- Codex: the access token's expiry is read from the JWT locally (never printed), so an expired login reads "login expired <time>" in `usage` / `plan` / `profiles` instead of a server 401.

## [0.2.1] - 2026-09-25

### Fixed

- Personal Claude account reported "login expired" while logged in: the keychain can hold two `Claude Code-credentials` entries (a stale one from an old Claude Code version with an empty token, and the live one). agx now reads the entry saved under the current macOS username — the account Claude Code writes — before falling back to any entry with that name.

### Removed

- v0.2.0 is retracted (`retract v0.2.0` in go.mod) and its GitHub release page removed; it contained the keychain bug above. Use v0.2.1.

## [0.2.0] - 2026-09-24

### Added

- `agx sessions move [folder] --to <profile>`: move a folder's conversations to another account of the same provider (e.g. personal → work) so they resume there. The target profile picks the provider; cross-provider and same-home moves are refused. Claude: transcripts, tool results, file checkpoints, environment snapshots and project memory move, merged into the target. Refuses while Claude runs in the folder or when an id already exists in the target; backs up the sources first (`--no-backup`); rolls back on a half-way failure; `--from`, `--only` (id or prefix), `--dry-run`, `--json`. Codex is refused with the reason (its SQLite index holds absolute paths).
- Provider interfaces `ConversationMover` and `MoveUnsupported`, and the shared `provider.ApplyMovePlan` / `CopyTree` mechanism.

### Changed

- A bare folder name given to `sessions promote` / `sessions move` is looked up under `sessions.root`, then in the current folder, then matched against the current folder's own name (typing the project's name from inside it no longer looks for `<name>/<name>`).
- After a full `sessions move`, the emptied history folder in the source account is removed (only if empty), so listing and promote no longer see stale "history" there.

## [0.1.0] - 2026-09-24


### Added

- `agx`, a toolkit CLI for AI coding agents (formerly `claude-usage`), built on voltkit (`appdir`, `output`, `versioncmd`) and cobra.
- Providers behind one interface: Claude Code and OpenAI Codex (`provider`, `provider/claude`, `provider/codex`, all importable).
- `agx usage` (also bare `agx`): plan usage for every account of every provider, deduplicated per login; `--provider`, `--json`, `--color`, `--timeout`.
- `agx profiles`: every configured or discovered profile with account, plan and login state.
- `agx run [-p profile|auto]`: launch on a profile, or on the account with the most headroom (`auto` scores each account by its fullest window).
- `agx new [slug…]`: timestamped session folder + launch; `agx resume [query]`: continue a conversation on the account whose home stores it, with fzf / numbered picker, `--last`, `--all`, `--list`, `--json`.
- `agx sessions ls | gc | promote`: list session folders, remove empty history-less ones safely, move one into a project together with its Claude history.
- `agx doctor`, `agx shell-init zsh|bash` (plan-file protocol so `new` / `resume` can `cd` the shell, plus aliases from config), `agx version`.
- Optional `~/.agx/config.yaml`: profiles, provider default args, env, `gopass:` / `env:` secrets resolved at launch, `billing: api` profiles, shell aliases. Zero-config discovery of `~/.claude`, `~/.claude-*` and `~/.codex`.
- `--dry-run` on `run` / `new` / `resume` prints the command with secret values redacted.
- Layering guard test (`internal/archtest`) and cross-compilation for macOS, Linux and Windows in `task check`.

- README: a full copy-paste Examples section (every command, `jq` recipes, config recipes), a `-p` guide and a flags-by-command reference.

### Fixed

- `agx resume` from a promoted project folder now finds conversations started before the move (it matched the stale path recorded inside the transcript; it now looks conversations up by history folder, as Claude Code does).
- `sessions promote` to another disk stops with "destination is on a different disk" instead of a raw rename error.
- A missing `security` / `gopass` binary given by absolute path is treated as not installed.
- HTTP 429 from a usage endpoint is reported as "usage endpoint rate-limited this account; try again shortly (retry after …)" instead of a raw status line, and error bodies are flattened to one line.

### Changed

- Minimum Go version is now 1.26 (required by voltkit).

[Unreleased]: https://github.com/khanakia/agx/compare/v0.3.3...HEAD
[0.3.3]: https://github.com/khanakia/agx/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/khanakia/agx/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/khanakia/agx/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/khanakia/agx/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/khanakia/agx/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/khanakia/agx/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/khanakia/agx/releases/tag/v0.1.0
