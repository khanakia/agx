# agx feature index

Every user-facing feature of agx, built from the code (`internal/cli`, `config`, `provider`, `login`, `sessions`, `pick`, `secret`, `internal/shellinit`) and grouped by area. Each row names the package that implements it, the guide section that explains it, and a runnable example program under [`examples/`](../../examples/README.md) that exercises the same API through the Go library ("—" means the feature is CLI-only or has no example yet). For the order to learn them in, see the [learning path](../README.md#learning-path).

## Contents

- [Usage and plans](#usage-and-plans)
- [Running and auto-pick](#running-and-auto-pick)
- [Resume and conversations](#resume-and-conversations)
- [Session folders](#session-folders)
- [Moving history](#moving-history)
- [Logins](#logins)
- [Configuration](#configuration)
- [Shell integration](#shell-integration)
- [Doctor and diagnostics](#doctor-and-diagnostics)
- [Output and JSON](#output-and-json)
- [Go library packages](#go-library-packages)
- [How this index is maintained](#how-this-index-is-maintained)

## Usage and plans

| Feature | Package | Guide | Example |
|---|---|---|---|
| Usage bars for every account (`agx`, `agx usage [profile…]`) | `internal/cli`, `provider/claude`, `provider/codex` | [Usage: show usage for every account](../guides/usage/usage-and-plans.md#step-1--show-usage-for-every-account) | [usage-fake-server](../../examples/usage-fake-server/) |
| Only one provider (`agx usage --provider claude\|codex`) | `internal/cli` | [Usage: narrow it down](../guides/usage/usage-and-plans.md#step-2--narrow-it-down) | — |
| Colour control (`--color auto\|always\|never`, `NO_COLOR`) | `internal/cli` | [Environment: colour](../guides/config-shell/environment.md#step-3--colour-no_color) | — |
| Per-account fetch timeout (`--timeout`, default 15s) | `internal/cli` | [Usage: narrow it down](../guides/usage/usage-and-plans.md#step-2--narrow-it-down) | — |
| Session, weekly and per-model windows, plus the synthesised "Extra usage" row | `provider`, `provider/claude` | [Usage: show usage for every account](../guides/usage/usage-and-plans.md#step-1--show-usage-for-every-account) | [usage-fake-server](../../examples/usage-fake-server/) |
| Rate-limited accounts reported per account (HTTP 429, retry-after), others still shown | `provider`, `internal/cli` | [Usage: read the error rows](../guides/usage/usage-and-plans.md#step-3--read-the-error-rows) | [usage-fake-server](../../examples/usage-fake-server/) |
| Exit status 0 / 1 / 2 for scripting | `internal/cli` | [JSON: branch on the exit status](../guides/usage/json-and-scripting.md#step-4--branch-on-the-exit-status) | — |
| Plan, subscription status and start date (`agx plan`, alias `agx subscription`) | `internal/cli`, `provider/claude`, `provider/codex` | [Usage: show plan and subscription](../guides/usage/usage-and-plans.md#step-4--show-plan-and-subscription) | — |
| `agx plan --login home` accepted, a token login refused | `internal/cli` | [Choosing a login: pitfalls](../guides/logins/choosing-a-login.md#pitfalls) | — |
| Profile listing (`agx profiles`, aliases `who`, `accounts`): home, account, state, login, billing, source | `internal/cli`, `config` | [Usage: list the profiles](../guides/usage/usage-and-plans.md#step-5--list-the-profiles) | [config-resolve](../../examples/config-resolve/) |
| Zero-config discovery of `~/.claude`, `~/.claude-<name>` and `~/.codex` | `provider/claude`, `provider/codex`, `config` | [Accounts: see what agx discovered](../guides/getting-started/accounts.md#step-1--see-what-agx-discovered) | [config-resolve](../../examples/config-resolve/) |

## Running and auto-pick

| Feature | Package | Guide | Example |
|---|---|---|---|
| Start the agent in the current folder (`agx run`) | `internal/cli`, `provider` | [Run: run on the default profile](../guides/running/run.md#step-1--run-on-the-default-profile) | [launch-command](../../examples/launch-command/) |
| Choose a profile (`-p/--profile NAME`) | `internal/cli`, `config` | [Run: pick a profile](../guides/running/run.md#step-2--pick-a-profile-with--p) | — |
| Auto-pick the account with the most headroom (`-p auto`) | `pick`, `internal/cli` | [Auto-pick: launch with -p auto](../guides/running/auto-pick.md#step-1--launch-with--p-auto) | [autopick](../../examples/autopick/) |
| Provider for the default / auto profile (`--provider`, default `claude`) | `internal/cli` | [Run: run on the default profile](../guides/running/run.md#step-1--run-on-the-default-profile) | — |
| Pass arguments to the agent after `--` (`run` only) | `internal/cli` | [Run: pass arguments to the agent](../guides/running/run.md#step-3--pass-arguments-to-the-agent) | [launch-command](../../examples/launch-command/) |
| Dry run with secrets and tokens redacted (`--dry-run` on run, new, resume, ws) | `internal/cli` | [Run: check what would run](../guides/running/run.md#step-4--check-what-would-run-with---dry-run) | — |
| Start in a fresh session folder (`agx new [slug…]`) | `internal/cli`, `sessions` | [Session folders: start in a new folder](../guides/running/new-and-sessions.md#step-1--start-in-a-new-folder) | [sessions-gc](../../examples/sessions-gc/) |
| Default home launched with `CLAUDE_CONFIG_DIR` / `CODEX_HOME` removed | `provider` | [Run: the default home rule](../guides/running/run.md#step-5--know-the-default-home-rule) | [launch-command](../../examples/launch-command/) |
| API-billed and third-party backends (`billing: api`, e.g. Kimi), never auto-picked | `config`, `pick` | [Run: an api-billed profile](../guides/running/run.md#step-6--run-an-api-billed-profile) | [autopick](../../examples/autopick/) |

## Resume and conversations

| Feature | Package | Guide | Example |
|---|---|---|---|
| Resume a conversation from this folder on its own account (`agx resume`) | `internal/cli`, `provider/claude`, `provider/codex` | [Resume: pick from this folder](../guides/running/resume.md#step-1--pick-from-this-folders-conversations) | [conversations](../../examples/conversations/) |
| Filter by title, folder or id prefix (`agx resume <words>`) | `internal/cli` | [Resume: filter with words](../guides/running/resume.md#step-2--filter-with-words) | — |
| Newest match without a picker (`--last`) | `internal/cli` | [Resume: skip the picker](../guides/running/resume.md#step-3--skip-the-picker-with---last) | — |
| Search every folder (`--all`) and cap the scan (`--limit`, default 50) | `internal/cli` | [Resume: search every folder](../guides/running/resume.md#step-4--search-every-folder-with---all) | [conversations](../../examples/conversations/) |
| Print instead of launching (`--list`, `--list --json`) | `internal/cli` | [Resume: print instead of launching](../guides/running/resume.md#step-5--print-instead-of-launching-with---list) | — |
| Override the resuming profile (`resume -p`) | `internal/cli` | [Resume: override the profile](../guides/running/resume.md#step-7--override-the-profile-with--p-advanced) | — |
| Workspace picker: any folder with chats, then a chat (`agx ws`, `resume -b/--browse`, `ws --list`) | `internal/cli` | [Workspace picker](../guides/running/workspace-picker.md) | — |
| Several profiles sharing one home: launch-profile choice and `profiles` in JSON | `config`, `internal/cli` | [Resume: which profile](../guides/running/resume.md#step-6--know-which-profile-a-conversation-resumes-on) | [config-resolve](../../examples/config-resolve/) |
| Fast transcript reading (folder from the head, title from the tail) | `provider/claude` | [Workspace picker: pitfalls](../guides/running/workspace-picker.md#pitfalls) | [conversations](../../examples/conversations/) |
| fzf picker with a numbered fallback when fzf is absent | `internal/cli` | [Resume: pitfalls](../guides/running/resume.md#pitfalls) | — |

## Session folders

| Feature | Package | Guide | Example |
|---|---|---|---|
| Session folder naming `[slug_]YYYYMMDD_HHMMSS` under `sessions.root` | `sessions` | [Session folders: start in a new folder](../guides/running/new-and-sessions.md#step-1--start-in-a-new-folder) | [sessions-gc](../../examples/sessions-gc/) |
| List session folders with file counts and history (`agx sessions ls`) | `sessions`, `internal/cli` | [Session folders: list the folders](../guides/running/new-and-sessions.md#step-2--list-the-folders) | [sessions-gc](../../examples/sessions-gc/) |
| Remove empty, history-less folders (`agx sessions gc`, `--yes`, `--min-age`) | `sessions`, `internal/cli` | [Session folders: clean up with gc](../guides/running/new-and-sessions.md#step-3--clean-up-with-gc) | [sessions-gc](../../examples/sessions-gc/) |
| Promote a folder into a project, carrying Claude history (`agx sessions promote`, `--to`, `--git-init`, `--dry-run`) | `sessions`, `provider/claude` | [Session folders: promote](../guides/running/new-and-sessions.md#step-4--promote-a-folder-into-a-project) | [sessions-gc](../../examples/sessions-gc/), [conversations](../../examples/conversations/) |
| Promote refuses a cross-disk destination (single atomic rename) | `sessions` | [Session folders: pitfalls](../guides/running/new-and-sessions.md#pitfalls) | — |

## Moving history

| Feature | Package | Guide | Example |
|---|---|---|---|
| Move a folder's conversations to another account (`agx sessions move --to`) | `provider`, `provider/claude`, `internal/cli` | [Move history: move](../guides/running/move-history.md#step-2--move) | [conversations](../../examples/conversations/) |
| Restrict the source and the conversations (`--from`, `--only` with id prefixes) | `internal/cli` | [Move history: move only some](../guides/running/move-history.md#step-3--move-only-some-conversations) | — |
| Interactive pick of chats and target (`-i/--pick`, `--all`) | `internal/cli` | [Move history: pick chats](../guides/running/move-history.md#step-4--pick-chats-instead-of-typing-ids) | — |
| Backup before moving (`--no-backup` to skip) and rollback on partial failure | `provider` | [Move history: move](../guides/running/move-history.md#step-2--move), [pitfalls](../guides/running/move-history.md#pitfalls) | — |
| Refuses while Claude runs in the folder, or when the target already holds a conversation | `provider/claude` | [Move history: pitfalls](../guides/running/move-history.md#pitfalls) | — |
| Same-provider only; Codex moves refused with a reason | `provider`, `provider/codex` | [Move history: pitfalls](../guides/running/move-history.md#pitfalls) | — |

## Logins

| Feature | Package | Guide | Example |
|---|---|---|---|
| Add a named login: hidden prompt, `--stdin`, `--setup` | `login`, `config`, `internal/cli` | [Add a stored login](../guides/logins/add-and-manage.md#step-2--add-a-stored-login) | [login-resolve](../../examples/login-resolve/) |
| Logins that store nothing (`--cmd`, `--env`) | `login` | [Add a login that stores nothing](../guides/logins/add-and-manage.md#step-3--add-a-login-that-stores-nothing) | [login-resolve](../../examples/login-resolve/) |
| API-key logins (`--kind api-key`) and account labels (`--account`) | `login`, `provider/claude` | [Pick a kind](../guides/logins/add-and-manage.md#step-4--pick-a-kind) | — |
| Token rotation (`agx login add NAME --replace`) | `login`, `config` | [Rotate a token](../guides/logins/add-and-manage.md#step-6--rotate-a-token) | — |
| List logins with expiry and users (`agx login ls`, alias `list`, `--json`) | `internal/cli`, `login` | [List and check](../guides/logins/add-and-manage.md#step-5--list-and-check) | [login-resolve](../../examples/login-resolve/) |
| Check logins by resolving and probing (`agx login check [NAME]`, `--no-probe`) | `internal/cli`, `provider/claude` | [Check a login before relying on it](../guides/logins/usage-and-auto.md#step-4--check-a-login-before-relying-on-it) | [login-resolve](../../examples/login-resolve/), [usage-fake-server](../../examples/usage-fake-server/) |
| Set a default login per profile or provider (`agx login use NAME\|home -p \| --provider`) | `config`, `internal/cli` | [Default for a profile](../guides/logins/choosing-a-login.md#step-3--make-it-the-default-for-a-profile) | — |
| Remove a login (`agx login rm`, alias `remove`, `--force`) | `config`, `login` | [Remove a login](../guides/logins/add-and-manage.md#step-9--remove-a-login) | — |
| Run on a login's quota (`--login NAME\|home` on run, new, resume, ws) | `internal/cli`, `provider` | [Use a login for one run](../guides/logins/choosing-a-login.md#step-2--use-a-login-for-one-run) | [launch-command](../../examples/launch-command/) |
| Auto-pick a login by probing (`--login auto`) | `internal/cli`, `pick`, `provider/claude` | [Let agx pick the login](../guides/logins/usage-and-auto.md#step-5--let-agx-pick-the-login-for-a-run) | — |
| Per-terminal login (`AGX_LOGIN`, `aglogin NAME`) | `internal/shellinit`, `internal/cli` | [Switch one terminal](../guides/logins/choosing-a-login.md#step-5--switch-one-terminal) | — |
| Login precedence: flag, `AGX_LOGIN`, profile, provider, home | `internal/cli` | [Learn the precedence](../guides/logins/choosing-a-login.md#step-1--learn-the-precedence) | — |
| Login usage by probe (`agx usage --logins`, `agx usage --login NAME`) | `provider/claude`, `internal/cli` | [See every login's usage](../guides/logins/usage-and-auto.md#step-2--see-every-logins-usage) | [usage-fake-server](../../examples/usage-fake-server/) |
| Private token file `~/.agx/tokens.yaml` (0600 in 0700, refused when readable by others) | `login` | [Keep tokens.yaml private](../guides/logins/add-and-manage.md#step-7--keep-tokensyaml-private) | [login-resolve](../../examples/login-resolve/) |
| Setup-token expiry: listed, warned 30 days ahead, stored logins refused when past | `login`, `provider/claude`, `internal/cli` | [Renew before expiry](../guides/logins/add-and-manage.md#step-8--renew-before-expiry) | [login-resolve](../../examples/login-resolve/) |
| Credential environment switch (`CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`, the others removed) | `provider/claude`, `provider` | [What changes in the agent's environment](../guides/logins/overview.md#step-4--understand-what-changes-in-the-agents-environment) | [launch-command](../../examples/launch-command/) |

## Configuration

| Feature | Package | Guide | Example |
|---|---|---|---|
| Config file `~/.agx/config.yaml`, directory overridable with `AGX_CONFIG_DIR` / `AGX_HOME` | `config`, `internal/cli` | [Environment: move the config folder](../guides/config-shell/environment.md#step-1--move-the-config-folder-agx_config_dir-and-agx_home) | [config-resolve](../../examples/config-resolve/) |
| Strict decoding: unknown keys are an error | `config` | [Config reference: pitfalls](../guides/config-shell/config-reference.md#pitfalls) | [config-resolve](../../examples/config-resolve/) |
| `sessions.root` and `sessions.promote_root` | `config` | [Config reference: sessions](../guides/config-shell/config-reference.md#sessions) | [config-resolve](../../examples/config-resolve/) |
| Provider defaults (`providers.<id>.args`, `providers.<id>.login`) | `config` | [Config reference: providers](../guides/config-shell/config-reference.md#providers) | — |
| Profiles: `name`, `provider`, `home`, `billing`, `default`, `args`, `args_replace`, `env`, `login` | `config` | [Config reference: profiles](../guides/config-shell/config-reference.md#profiles) | [config-resolve](../../examples/config-resolve/) |
| Secrets resolved at launch (`secrets:` with `gopass:<path>` or `env:<NAME>`) | `secret`, `config` | [Config reference: profiles](../guides/config-shell/config-reference.md#profiles) | [secret-refs](../../examples/secret-refs/) |
| Logins in config (`name`, `provider`, `kind`, `token: stored`, `token_cmd`, `token_env`, `account`, `added`); raw tokens rejected | `config`, `login` | [Config reference: logins](../guides/config-shell/config-reference.md#logins) | — |
| Discovery switched off per provider once a profile is configured; config order breaks auto-pick ties | `config`, `pick` | [Config reference: discovery](../guides/config-shell/config-reference.md#step-3--understand-discovery), [Auto-pick: ties](../guides/running/auto-pick.md#step-4--steer-ties-with-config-order) | [config-resolve](../../examples/config-resolve/), [autopick](../../examples/autopick/) |
| Comment- and order-preserving config edits by `agx login add` / `use` / `rm` | `config` | [Config reference: let agx edit the file](../guides/config-shell/config-reference.md#step-4--let-agx-edit-the-file-for-you) | — |
| Ready-made recipes (two accounts, extra accounts, Kimi, aliases) | `config` | [Config reference: a complete file](../guides/config-shell/config-reference.md#step-1--start-from-a-complete-file) | — |

## Shell integration

| Feature | Package | Guide | Example |
|---|---|---|---|
| Shell layer (`agx shell-init zsh\|bash`) | `internal/shellinit` | [Shell integration: load the layer](../guides/config-shell/shell-integration.md#step-1--load-the-layer) | — |
| Land in the session folder after new, resume, ws (launch-plan protocol via `AGX_PLAN_FILE` and hidden `agx exec`) | `internal/shellinit`, `internal/cli` | [Shell integration: the launch-plan protocol](../guides/config-shell/shell-integration.md#step-2--understand-the-launch-plan-protocol) | — |
| Report the folder to the terminal (OSC 7) | `internal/shellinit` | [Shell integration: the launch-plan protocol](../guides/config-shell/shell-integration.md#step-2--understand-the-launch-plan-protocol) | — |
| Alias functions from `shell.aliases` (values limited to a safe character set) | `internal/shellinit`, `config` | [Shell integration: add aliases](../guides/config-shell/shell-integration.md#step-3--add-aliases) | [config-resolve](../../examples/config-resolve/) |
| `aglogin` function (backed by hidden `agx login exists`) | `internal/shellinit` | [Shell integration: aglogin](../guides/config-shell/shell-integration.md#step-4--switch-logins-per-terminal-with-aglogin) | — |
| Tab completion (`agx completion zsh\|bash\|fish`) | `internal/cli` | [Shell integration: tab completion](../guides/config-shell/shell-integration.md#step-5--tab-completion) | — |
| Version and build provenance (`agx version`, `--json`, `agx --version`) | `internal/cli` | [Getting started: check the version](../guides/getting-started/README.md#step-3--check-the-version) | — |

## Doctor and diagnostics

| Feature | Package | Guide | Example |
|---|---|---|---|
| Local health check (`agx doctor`, `--json`), no network, exit 1 on any failure | `internal/cli` | [Doctor: run it](../guides/usage/doctor.md#step-1--run-it) | — |
| Which rung decided the config directory | `internal/cli` | [Doctor: run it](../guides/usage/doctor.md#step-1--run-it) | — |
| Per profile: secret refs parse, gopass on PATH, agent binary on PATH, home exists, login state | `internal/cli`, `secret` | [Doctor: profile NAME](../guides/usage/doctor.md#profile-name) | — |
| tokens.yaml: permissions, folder mode, inside a git work tree | `internal/cli`, `login` | [Doctor: tokens.yaml](../guides/usage/doctor.md#tokensyaml) | — |
| Per login: token present, `--env` variable set, expiry state | `internal/cli`, `login` | [Doctor: login NAME](../guides/usage/doctor.md#login-name) | — |
| Shell layer active (`AGX_SHELL_INIT`) | `internal/cli`, `internal/shellinit` | [Doctor: shell](../guides/usage/doctor.md#shell) | — |

## Output and JSON

| Feature | Package | Guide | Example |
|---|---|---|---|
| Stable JSON envelope (`schema_version`, `kind`, `count`, `data`) on every listing | `internal/cli` | [JSON: know the envelope](../guides/usage/json-and-scripting.md#step-1--know-the-envelope) | — |
| JSON kinds: `usage.list`, `plan.list`, `profile.list`, `conversation.list`, `workspace.list`, `session.list`, `session.gc`, `session.promote`, `session.move`, `login.list`, `doctor.report`, `version.show` | `internal/cli` | [JSON: read the kind you need](../guides/usage/json-and-scripting.md#step-2--read-the-kind-you-need) | — |
| `jq` recipes for status bars and scripts | — | [JSON: the jq recipes](../guides/usage/json-and-scripting.md#step-3--use-the-jq-recipes) | — |
| `--help` with examples on every command | `internal/cli` | [Troubleshooting: a command refuses its arguments](../guides/usage/troubleshooting.md#step-4--a-command-refuses-its-arguments) | — |

## Go library packages

| Feature | Package | Guide | Example |
|---|---|---|---|
| Vendor-neutral model: `Profile`, `Usage`, `Window`, `Conversation`, `Command`, `Provider` and optional upgrades (`ConversationLister`, `HistoryMover`, `HistoryChecker`, `SubscriptionReader`, `ConversationMover`, `LoginLauncher`, `LoginUsageReader`) | `provider` | [Provider model: implement the interface](../guides/library/provider-model.md#step-1--implement-the-required-interface) | [custom-provider](../../examples/custom-provider/) |
| Environment helpers (`HomeEnv`, `SetEnv`, `UnsetEnv`, `ApplyLoginEnv`) and `RateLimitError` | `provider` | [Provider model: HomeEnv](../guides/library/provider-model.md#step-3--build-the-launch-command-with-homeenv) | [custom-provider](../../examples/custom-provider/), [launch-command](../../examples/launch-command/), [usage-fake-server](../../examples/usage-fake-server/) |
| Move plans (`MovePlan`, `ApplyMovePlan` with backup and rollback, `CopyTree`) | `provider` | [Claude Code and Codex: move history](../guides/library/claude-and-codex.md#step-4--move-history) | [conversations](../../examples/conversations/) |
| Claude Code: discovery, credentials (file + Keychain), usage, subscription, conversations, `EncodePath`, launch, probe | `provider/claude` | [Claude Code and Codex providers](../guides/library/claude-and-codex.md#step-1--read-usage-from-a-homes-login) | [usage-fake-server](../../examples/usage-fake-server/), [conversations](../../examples/conversations/), [launch-command](../../examples/launch-command/) |
| Codex: discovery, usage, subscription, conversations, launch | `provider/codex` | [Claude Code and Codex: launch commands](../guides/library/claude-and-codex.md#step-5--build-launch-commands) | [launch-command](../../examples/launch-command/), [config-resolve](../../examples/config-resolve/) |
| Load, decode and resolve the YAML config against any providers; the `Editor` for login edits | `config` | [Config, pick and sessions: load and resolve](../guides/library/config-pick-sessions.md#step-1--load-and-resolve-the-config) | [config-resolve](../../examples/config-resolve/) |
| Auto-pick policy (`pick.Best`, `Result.Explain`) | `pick` | [Config, pick and sessions: pick.Best](../guides/library/config-pick-sessions.md#step-2--choose-a-profile-with-pickbest) | [autopick](../../examples/autopick/) |
| Session folders (`Slug`, `Name`, `Create`, `List`, `PlanGC`, `RemoveEmpty`, `Promote`) | `sessions` | [Config, pick and sessions: session folders](../guides/library/config-pick-sessions.md#step-3--create-list-and-collect-session-folders) | [sessions-gc](../../examples/sessions-gc/) |
| Secret references (`secret.Parse`, `System.Resolve`, `ResolveAll`) | `secret` | [Secrets and logins: secret references](../guides/library/secrets-and-logins.md#step-1--parse-and-resolve-secret-references) | [secret-refs](../../examples/secret-refs/) |
| Named logins (`login.Resolver`, `Clean`, `ExpiresAt`, `Compatible`, `TokenFile`) | `login` | [Secrets and logins: resolve a login](../guides/library/secrets-and-logins.md#step-3--resolve-a-login-and-apply-it-to-a-launch) | [login-resolve](../../examples/login-resolve/) |
| Enforced layering (domain model imports only the standard library) | `internal/archtest` | [Library: respect the layering](../guides/library/README.md#step-2--respect-the-layering) | — |

## How this index is maintained

This page lists features, not every flag spelling; the full flag tables are in [README: Flags by command](../../README.md#flags-by-command). When a command, flag, config key, environment variable, doctor check or exported library function is added or changed, add or update its row here in the same change, and point Guide at the guide section that explains it. Example names a program under `examples/` only when that program calls the feature's API (check with `grep` in its `main.go`); it stays "—" otherwise. `task docs:lint` fails on any Guide or Example link that does not resolve to an existing file and heading.
