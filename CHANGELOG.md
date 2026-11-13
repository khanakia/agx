# Changelog

All notable changes to **agx** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- agx now locates its config folder with `github.com/ubgo/appdir` v0.1.0 instead of `github.com/ubgo/voltkit/appdir`, which is retired. The folder is unchanged for every user: `AGX_CONFIG_DIR`, then `AGX_HOME/config`, then `~/.agx`, for every build, and `agx doctor` names the same rules. Nothing is moved or created.

## [0.3.9] - 2026-11-09

### Added

- The usage login table (`agx usage --logins --table`, `--login a,b`, `--login all`, `--pick`) gains `RESET DAY` (e.g. `Tue`) and `RESETS ON` (e.g. `Tue 10 Nov 05:33`) right after `WEEKLY RESETS`, from the weekly window's reset time (`—` when a row has none), and a line under the table, `Weekly resets: Tue 1 (+79%) · Wed 2 (+164%)`: per weekday, how many logins' weekly windows reset then and the sum of their weekly USED (the room they get back), soonest reset first.
- `agx usage --sort KEYS` orders the table's rows by comma-separated keys `login`, `left`, `5h`, `5h-reset`, `weekly`, `weekly-reset`, `day`, `status`; a `-` prefix sorts high to low (`--sort day,-left`). `day` is calendar order Mon..Sun, `weekly-reset` the actual reset time, soonest first, and a row without the value goes last in either direction. The sort is stable and `#` keeps each login's own number. It works only with the table; an unknown key is a usage error listing the valid ones.
- `agx usage --tz ZONE` (an IANA name such as `Asia/Kolkata`; default: the machine's local zone) sets the zone of `RESET DAY`, `RESETS ON`, the summary line and `--sort day`. An unknown zone is a usage error.
- `usage.list` JSON gains `weekly_resets_at` (RFC3339 in the `--tz` zone) and `weekly_reset_day` (e.g. `Tue`) on every entry with a weekly window.

## [0.3.8] - 2026-11-04

### Added

- `agx usage --logins --table`: one compact row per token login instead of a block of bars, with the columns `#  LOGIN  PROVIDER  ACCOUNT  5H USED  5H RESETS  WEEKLY USED  WEEKLY RESETS  LEFT  STATUS` (plus a `<MODEL> WEEKLY` pair when a probe returns a per-model window). `#` is the login's position in `agx login ls`; a window the account lacks is `—`, never `0%`; LEFT is 100 minus the fullest window; STATUS is `ok`, `limit reached`, `rejected` (with `blocked by org` or the HTTP status), `expired`, `per token` or `error` with a short reason, and the full message of each failed row prints under the table. All logins are probed at once with the per-account `--timeout`; the command exits 1 when any row failed.
- `agx usage --login a,b` and `--login all` print that table for the named logins (or every login), in `agx login ls` order with duplicates dropped; `home` and `auto` are refused inside a list and an unknown name is a usage error.
- `agx usage --pick`: choose token logins in a checklist (space marks, `a` all/none, Enter shows, Esc/`q` cancels with exit 1), then the table. It needs a terminal on stdin and stdout.
- `usage.list` JSON gains `status`, `status_reason`, `left_percent` and `left_resets_at` on every entry, home or login, with or without `--table`.

## [0.3.7] - 2026-11-04

### Added

- Every launch (`run`, `new`, `resume`, `ws`, a bare `agx` in a bound folder, the shell layer's plan, `profile add --setup`) sets `AGX_RUN_PROFILE`, `AGX_RUN_LOGIN` (a login name, or `home`) and `AGX_RUN_HOME` in the agent's environment, replacing any inherited values so a nested launch reports its own. They are outputs only: agx never reads them, so they cannot change which profile or login a launch picks, and they never hold a token. `--dry-run` lists them with the other environment changes. `AGX_LOGIN` is still never set for the agent.
- `agx whoami`: the profile, login and home of the current session, from those variables, plus the home each provider's CLI would use here and whether a token login variable is present (names only, never values). Outside an agx launch it says `not inside a session started by agx`. Reads no config and no network; `--json` (`whoami.report`).
- Library: the optional `provider.HomeEnvNamer` upgrade, implemented by `claude` (`CLAUDE_CONFIG_DIR`) and `codex` (`CODEX_HOME`).

### Fixed

- A login probe (`agx usage --login NAME`, `--logins`, `--login auto`, `agx login check`) no longer fails with `anthropic-ratelimit-unified-5h-utilization missing` on an account whose plan has no 5-hour window: a window whose headers are all absent is left out, and the probe errors only when no window comes back at all. A window with only half of its headers is still an error.

## [0.3.6] - 2026-11-01

### Added

- `agx profile add NAME`: creates the home (`~/.claude-NAME`, or `~/.codex-NAME` with `--provider codex`; `--home` for another folder) with mode 0700 and adds the profile to config.yaml, keeping comments. A new Claude home links `CLAUDE.md`, `skills` and `settings.json` from `~/.claude` when they exist there; nothing is ever overwritten, `--bare` links nothing, and a home that already exists is adopted untouched. `--login` sets the profile's login, `--setup` runs `claude auth login` (or `codex login`) for the new home, `--dry-run` prints the plan.
- `agx project use [-p PROFILE] [--login LOGIN]`, `show`, `ls`, `rm`: folder bindings, stored in config.yaml under `projects:` keyed by the folder's absolute path with symlinks resolved, never in the repo. The longest bound path (by whole folder names) covers a folder and everything under it. Profile and login resolve independently: profile from `-p`, the binding, the default profile; login from `--login`, `AGX_LOGIN`, the binding, the profile's `login:`, the provider default, the home. `resume` keeps the conversation's home and takes only the binding's login, from the conversation's folder. `show` explains which rule chose each part; `ls` and `show` have `--json` (`project.list`, `project.show`).
- A bare `agx` in a bound folder, with a terminal on stdin and stdout and no flags, starts the agent like `agx run` after one stderr line (`agx: project PATH -> profile NAME, login NAME`). Piped, with flags, or in an unbound folder it shows usage as before; `agx usage` always does.
- `agx doctor` checks every binding (a removed profile or login, a login that cannot run the bound profile, a folder that no longer exists); `agx login ls` lists bindings under USED-BY, and `agx login rm` refuses while a binding uses the login (`--force` clears it, removing a binding left with nothing to bind).
- Token kind detection: `agx login add` sets the kind from a Claude token's prefix (`sk-ant-oat…` oauth-token, `sk-ant-api…` api-key), refuses a contradicting `--kind`, and refuses a refresh token (`sk-ant-ort…`) or an Admin API key (`sk-ant-admin…`). The same check runs on every resolution (the add check, `agx login check`, each launch), so `--cmd` and `--env` sources are covered. Library: the optional `provider.LoginKindDetector` upgrade.
- Library: `config.Project`, `config.MatchProject`, `config.CanonicalPath`, `config.ExpandPath`, `config.ValidProfileName`, `Config.ProjectFor`, `Editor.AddProfile`, `Editor.SetProject`, `Editor.RemoveProject`; `provider.HomeSetup`, `provider.HomeLink`, `provider.PlanHomeLinks`, `provider.ApplyHomeLinks`, `provider.ErrInferenceOnly`; `claude.ErrUnusableToken`.

### Changed

- A 403 from the Claude usage or profile endpoint whose `error.details.error_code` is `oauth_scope_insufficient` (a setup-token) now reports `provider.ErrInferenceOnly` with a hint to use `agx usage --login NAME`, instead of "log in again". Other 401/403 answers carry the server's message, for example `login rejected by the server (HTTP 403: OAuth authentication is currently not allowed for this organization.)`.
- The login precedence gains the project binding after `AGX_LOGIN`; a launch that a binding chose says so in `--dry-run` (`project:` line, `login: NAME (project binding at PATH)`), and the launch plan of `new` / `resume` / `ws` carries the binding's login.

### Fixed

- `agx login check` and the check after `agx login add` name a failing login once (`login NAME: failed: reason`); every login resolution error now reads `login NAME: reason`.

## [0.3.5] - 2026-10-17

### Fixed

- A stored login named `null` or `~` now reads back from `tokens.yaml`: its key is written quoted, where before it decoded as a YAML null and the token was lost.
- A Claude usage probe whose headers carry a `NaN` or `Inf` utilization is refused, instead of the garbled window being skipped and the account looking idle to auto-pick.
- `agx sessions gc --yes` no longer lists or counts a folder as removed when its removal failed; the exit code and `--json` output are unchanged.
- Codex plan names that start with a non-ASCII letter keep valid UTF-8 in their label (the first character, not the first byte, is upper-cased).

### Tests

- Input-space matrix tests across every package (coverage 89.8% to 92.9%), which found the fixes above.

## [0.3.4] - 2026-10-17

### Fixed

- `agx resume -p <name>` with an unknown profile now says `unknown profile "<name>"` and exits 2, instead of "no profile uses ~/.claude" and exit 1.
- `agx doctor` lists a profile's secret findings in a stable order, so its text and `--json` output no longer change from run to run.
- Replacing a stored login with `--cmd` or `--env` (`agx login add --replace`) now removes the old raw token from `tokens.yaml`.
- Error messages that quote a server response are cut on a character boundary and never end in half a UTF-8 character.
- `agx new`, `agx resume` and `agx ws` pass the words after `--` to the agent, as `agx run` does. Before, `new` folded them into the folder name and `resume` / `ws` into the query, and the agent got none.
- `agx new --login NAME` checks the login before it creates the session folder, so an unknown or unusable login no longer leaves an empty folder behind.
- `--login` help reads `--login string` instead of `--login agx login ls`.
- `agx login add --replace` of a stored login puts the previous token back in tokens.yaml when the config edit is refused, instead of losing it.
- `agx doctor` prints one warning row for a `token_env` login whose variable is unset, not a warning followed by a contradictory `no known expiry` row.
- `agx doctor` warns when `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set in the shell for a plan-billed Claude profile on its home login: Claude Code would bill the API instead of the plan. agx still passes the environment through unchanged.
- `agx sessions move` gives each backup its own folder (`-2`, `-3`, … after the time stamp) when two moves happen in the same second; the second move used to fail on files the first had already copied.
- `agx sessions promote` no longer leaves empty parent folders behind when it refuses a cross-disk destination or the move fails.
- The `sessions` library treats a missing history check as "unknown" and `PlanGC` keeps those folders (reason `history unknown`), instead of marking folders that have conversations as removable. New: `sessions.Folder.HistoryUnknown`, `sessions.GCHistoryUnknown`.
- `agx sessions move --only` with a prefix that matches several conversations says `ambiguous conversation prefix … matches N conversations` (`claude.ErrAmbiguousID`) instead of "no conversation".
- Multi-line tokens from any source (`--stdin`, a paste, a stored token, a command, an env variable) are refused with `token has more than one line` instead of a message about a token command.
- A profile `login:` that conflicts with the profile's own credential no longer prints a doubled `login: login:` prefix.
- `agx login use home --provider …` and `agx login rm` no longer leave an empty `providers: claude: {}` in config.yaml.
- Secret resolution and login environment variables are processed in sorted order, so errors and the launch environment are the same on every run.
- Conversation titles are found even when the transcript JSON has spaces after the colon (`"type": "ai-title"`).

### Documentation

- New `docs/` tree: an index with a fourteen-step learning path, a "start from your goal" table and the architecture diagram (`docs/README.md`); 24 step-by-step guides under `docs/guides/` (getting started, usage and diagnostics, running, logins, configuration and shell, the Go library), each with complete snippets, real output and pitfalls traced to the code; four decision pages under `docs/choosing/` (which profile or login, which token source, running and resuming, library or CLI); and a feature index (`docs/reference/features.md`) that points every feature at its guide section and, for 46 of 95, a runnable example.
- `examples/`: nine Go programs that use the library against temporary homes and `httptest` servers (usage and probe, auto-pick, config resolution, conversations and history moves, launch commands, logins, secrets, session folders, a custom provider). `examples/examples_test.go` builds and runs each one and checks its output, so `go test ./...` fails when an example drifts.
- Docs tooling in `scripts/docs/`: `task docs:snippets` regenerates the Go code and program output quoted in the docs from the real sources, `task docs:snippets:check` fails when a quote is stale, `task docs:lint` fails on hard-wrapped prose, broken relative links or anchors, machine-local paths and authorship wording, and `task docs:all` runs these with `task docs:check`.

## [0.3.3] - 2026-10-16

### Added

- Logins: run any home on any account's quota. A login is a named, long-lived Claude token (`claude setup-token`) or API key that agx passes to Claude Code at launch, so a home keeps its history, settings and `resume` while the run spends another account's plan. `agx login add NAME` takes the token at a hidden prompt, from `--stdin`, after `--setup` (runs `claude setup-token` first), or as a source read at each launch: `--cmd "<command>"` or `--env VAR`. `--kind api-key` adds an API key instead; `--account` is a label; `--replace` rotates a token and keeps the kind and account.
- `agx login ls` (`--json`, kind `login.list`; never a value), `agx login check [NAME]` (resolves the token and probes its usage; `--no-probe` only resolves), `agx login use NAME|home -p PROFILE` / `--provider ID` (edits config.yaml in place, keeping comments and order, and prints the key before and after; a discovered profile is first written into the file together with its provider's other discovered profiles) and `agx login rm NAME` (refused while a profile or provider default uses it; `--force` clears those references; prints that removing does not revoke the token).
- `--login NAME|home|auto` on `run`, `new`, `resume` and `ws`, carried through the shell layer's plan file. Precedence: `--login`, then `AGX_LOGIN`, then the profile's `login:`, then `providers.<id>.login`, then the home's own login. A login that does not fit the profile (another provider, `billing: api`, or a profile with its own credential in `env:` / `secrets:`) is an error when asked for by name and skipped with a note when it comes from `AGX_LOGIN`.
- `aglogin NAME` in `agx shell-init` (zsh and bash): exports `AGX_LOGIN` for this terminal after checking the name; `aglogin` with no argument unsets it.
- Config keys `logins:` (`name`, `provider`, `kind`, exactly one of `token: stored` / `token_cmd` / `token_env`, `account`, `added`), a profile's `login:` and `providers.<id>.login`. A raw token pasted into config.yaml is rejected without echoing it.
- `~/.agx/tokens.yaml` for pasted tokens: mode 0600 in a 0700 folder, atomic writes, refused when group or others can access it (with the `chmod 600` that fixes it).
- At launch an `oauth-token` login sets `CLAUDE_CODE_OAUTH_TOKEN` and removes `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN`; an `api-key` login sets `ANTHROPIC_API_KEY` and removes the other two. The launch line says `agx: <profile> · login <name>`, and `--dry-run` shows `login:` and `NAME=<token:source>`. A missing, empty or multi-line token, a failing or hung `--cmd` (10 s limit) or an unset `--env` stops the launch; agx never falls back to the home login silently.
- Usage of a login by probe: a setup-token cannot read the usage endpoint (403), so agx sends one minimal Messages request (smallest model, one output token) and reads the `anthropic-ratelimit-unified-5h-*` / `-7d-*` response headers into the same 5-hour and weekly windows. A probe spends a few tokens of the login's plan quota and counts toward its rate limits, so it runs only on request: `agx usage --logins` (one row per login after the homes; JSON rows carry `login`, `account` and `"source": "probe"`, kind unchanged), `agx usage --login NAME`, `agx login check` and `--login auto`. A plain `agx` / `agx usage` never probes. Missing or malformed headers are an error, never a 0% row; api-key logins are never probed. An account at its limit answers the probe with HTTP 429 but still sends the headers, so it shows as 100% used with its reset time rather than as rate-limited.
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

- `agx`, a toolkit CLI for coding agents (formerly `claude-usage`), built on voltkit (`appdir`, `output`, `versioncmd`) and cobra.
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

[Unreleased]: https://github.com/khanakia/agx/compare/v0.3.9...HEAD
[0.3.9]: https://github.com/khanakia/agx/compare/v0.3.8...v0.3.9
[0.3.8]: https://github.com/khanakia/agx/compare/v0.3.7...v0.3.8
[0.3.7]: https://github.com/khanakia/agx/compare/v0.3.6...v0.3.7
[0.3.6]: https://github.com/khanakia/agx/compare/v0.3.5...v0.3.6
[0.3.5]: https://github.com/khanakia/agx/compare/v0.3.4...v0.3.5
[0.3.4]: https://github.com/khanakia/agx/compare/v0.3.3...v0.3.4
[0.3.3]: https://github.com/khanakia/agx/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/khanakia/agx/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/khanakia/agx/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/khanakia/agx/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/khanakia/agx/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/khanakia/agx/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/khanakia/agx/releases/tag/v0.1.0
