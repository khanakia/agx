# Changelog

All notable changes to **agx** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/khanakia/agx/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/khanakia/agx/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/khanakia/agx/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/khanakia/agx/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/khanakia/agx/releases/tag/v0.1.0
