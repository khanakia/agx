# agx documentation

agx is one command for Claude Code and Codex across every account on your machine: plan usage for all of them, launching on the account with the most headroom, resuming a conversation on the account that stores it, session folders, moving history between accounts, and named logins that let any home spend any account's quota. These pages take you from the first install to a full setup, then serve as the reference. The [project README](../README.md) is the one-page overview.

## Learning path

Follow the steps in order the first time; each builds on the one before. "Example" is a runnable Go program under [`examples/`](../examples/README.md) that exercises the same feature through the library ("—" where the feature is CLI-only).

| # | Step | What you learn | Guide | Example |
|---|---|---|---|---|
| 1 | Install | download a release and check its checksum, or `go install`; check the version | [Install agx and run it the first time](guides/getting-started/README.md#step-1--download-a-release-and-check-its-checksum) | — |
| 2 | First run | the three first-run commands: `agx`, `agx profiles`, `agx doctor` | [Run the first-run commands](guides/getting-started/README.md#step-4--run-the-three-first-run-commands) | [config-resolve](../examples/config-resolve/) |
| 3 | Accounts | what agx discovers (`~/.claude`, `~/.claude-<name>`, `~/.codex`), adding a second Claude Code or Codex account, naming profiles | [Find and add accounts](guides/getting-started/accounts.md) | [config-resolve](../examples/config-resolve/) |
| 4 | Usage | usage windows per account, error rows, `agx plan`, `agx profiles` | [Usage, plans and profiles](guides/usage/usage-and-plans.md) | [usage-fake-server](../examples/usage-fake-server/) |
| 5 | Run and auto-pick | `agx run`, `-p`, arguments after `--`, `--dry-run`, the default-home rule, `-p auto` and its score | [Start an agent with agx run](guides/running/run.md), [Auto-pick](guides/running/auto-pick.md) | [launch-command](../examples/launch-command/), [autopick](../examples/autopick/) |
| 6 | Resume and workspaces | `agx resume` (query, `--last`, `--all`, `--list`), which profile a chat resumes on, `agx ws` | [Resume a conversation](guides/running/resume.md), [Workspace picker](guides/running/workspace-picker.md) | [conversations](../examples/conversations/) |
| 7 | Session folders | `agx new`, `agx sessions ls`, `gc`, `promote` | [Session folders](guides/running/new-and-sessions.md) | [sessions-gc](../examples/sessions-gc/) |
| 8 | Move history | moving a folder's conversations to another Claude account, backup, rollback, undo; why Codex is refused | [Move a folder's history](guides/running/move-history.md) | [conversations](../examples/conversations/) |
| 9 | Logins | home vs login, adding token and API-key logins, precedence, `aglogin`, login usage by probe, `--login auto` | [Logins overview](guides/logins/overview.md), then [Add and manage](guides/logins/add-and-manage.md), [Choosing a login](guides/logins/choosing-a-login.md), [Usage and auto](guides/logins/usage-and-auto.md) | [login-resolve](../examples/login-resolve/), [launch-command](../examples/launch-command/) |
| 10 | Config reference | every key of `~/.agx/config.yaml`, discovery, secrets, edits by `agx login`, the environment variables | [Config reference](guides/config-shell/config-reference.md), [Environment variables](guides/config-shell/environment.md) | [config-resolve](../examples/config-resolve/), [secret-refs](../examples/secret-refs/) |
| 11 | Shell integration | landing in the session folder, the launch-plan protocol, aliases, `aglogin`, completion | [Shell integration](guides/config-shell/shell-integration.md) | — |
| 12 | Doctor and troubleshooting | every `agx doctor` check and its fix, the error messages and what they mean | [Health check](guides/usage/doctor.md), [Troubleshooting](guides/usage/troubleshooting.md) | — |
| 13 | Scripting | the JSON envelope and every kind, `jq` recipes, exit codes | [JSON output and scripting](guides/usage/json-and-scripting.md) | — |
| 14 | Library | the packages, the layering, implementing your own provider | [Use agx as a Go library](guides/library/README.md), then [Provider model](guides/library/provider-model.md) | [custom-provider](../examples/custom-provider/) and [all examples](../examples/README.md) |

Unsure which of two similar options you need? The [choosing guides](choosing/README.md) compare them side by side.

## How the pieces fit

![agx architecture: you run a shell alias or an agx command in the terminal; internal/cli reads flags and ~/.agx/config.yaml, resolves the profile and login through the config, login and secret packages, picks the account with the most headroom when asked, builds the command and environment through the vendor-neutral provider model and the provider/claude and provider/codex implementations, and finally execs claude or codex in the chosen home; usage windows and conversation lists flow back and are rendered as tables or --json output. agx doctor, shell-init, sessions and login are the side commands around this flow.](assets/architecture.svg)

The numbered flow: ① a shell alias or ② an `agx` command → ③ config, login and secret resolve the profile and login → ④ pick chooses an account for `auto` → ⑤ the provider builds the command and env → ⑥ agx execs Claude Code or Codex in the chosen home. Layering is enforced by a test; see [Respect the layering](guides/library/README.md#step-2--respect-the-layering).

## Start from your goal

| I want to | Read |
|---|---|
| See how much of every plan I have left | [Usage, plans and profiles](guides/usage/usage-and-plans.md#step-1--show-usage-for-every-account) |
| Add my work account next to my personal one | [Find and add accounts](guides/getting-started/accounts.md#step-2--add-a-second-claude-code-account) |
| Start on whichever account has the most room | [Auto-pick](guides/running/auto-pick.md#step-1--launch-with--p-auto) |
| Continue yesterday's chat without remembering the account | [Resume](guides/running/resume.md#step-1--pick-from-this-folders-conversations) |
| Find a chat when I do not remember the folder | [Workspace picker](guides/running/workspace-picker.md#step-1--browse-workspaces-then-chats) |
| Keep experiments out of my projects | [Session folders](guides/running/new-and-sessions.md#step-1--start-in-a-new-folder) |
| Turn an experiment into a project | [Promote a folder](guides/running/new-and-sessions.md#step-4--promote-a-folder-into-a-project) |
| Move a project's chats from my personal to my work account | [Move history](guides/running/move-history.md#step-1--see-the-plan-with---dry-run) |
| Keep my home but spend another account's quota | [Logins overview](guides/logins/overview.md) and [Which profile, which login](choosing/which-profile-or-login.md) |
| Keep tokens in my password manager | [Which token source](choosing/which-token-source.md) |
| Use Kimi or another per-token backend | [Run an api-billed profile](guides/running/run.md#step-6--run-an-api-billed-profile) |
| Get `cd` into the new folder after `agx new` | [Shell integration](guides/config-shell/shell-integration.md#step-1--load-the-layer) |
| Put usage in my tmux or status bar | [jq recipes](guides/usage/json-and-scripting.md#step-3--use-the-jq-recipes) |
| Fix "not logged in", a config error or a refused command | [Troubleshooting](guides/usage/troubleshooting.md) |
| Check the whole setup | [Health check](guides/usage/doctor.md#step-1--run-it) |
| Decide between a script and a Go program | [Library or CLI](choosing/library-or-cli.md) |
| Add support for another agent CLI | [The provider model](guides/library/provider-model.md) |
| Find every feature and where it is documented | [Feature index](reference/features.md) |

## Every page

### Getting started

- [Install agx and run it the first time](guides/getting-started/README.md): release download and checksum, `go install`, version, first-run commands, optional shell layer.
- [Find and add accounts](guides/getting-started/accounts.md): discovery, a second Claude Code or Codex account, naming profiles.

### Usage and diagnostics

- [Usage, plans and profiles](guides/usage/usage-and-plans.md): `agx usage`, provider filter, colour, timeout, error rows, `agx plan`, `agx profiles`.
- [JSON output and scripting](guides/usage/json-and-scripting.md): the envelope, every kind's fields, `jq` recipes, exit codes.
- [Health check: agx doctor](guides/usage/doctor.md): every check, its level and its fix.
- [Troubleshooting](guides/usage/troubleshooting.md): the messages agx prints, what they mean, the fix.

### Running

- [Start an agent with agx run](guides/running/run.md): `-p`, `--provider`, `--`, `--dry-run`, the default-home rule, api-billed profiles.
- [Auto-pick the account with the most headroom](guides/running/auto-pick.md): `-p auto`, the score, ties, the same policy from Go.
- [Resume a conversation](guides/running/resume.md): queries, `--last`, `--all`, `--limit`, `--list`, the resuming profile, `-p`.
- [Workspace picker: agx ws](guides/running/workspace-picker.md): browse workspaces, then chats; `ws --list`.
- [Session folders: agx new and agx sessions](guides/running/new-and-sessions.md): naming, `ls`, `gc`, `promote`.
- [Move a folder's history: agx sessions move](guides/running/move-history.md): plan, move, `--only`, `-i`, undo, refusals.

### Logins

- [Logins overview](guides/logins/overview.md): home vs login, what changes in the agent's environment.
- [Add and manage logins](guides/logins/add-and-manage.md): sources, kinds, `ls`, `check`, rotation, `tokens.yaml`, expiry, removal.
- [Choosing a login](guides/logins/choosing-a-login.md): the precedence, per-run, per-profile, per-provider and per-terminal choices.
- [Login usage and --login auto](guides/logins/usage-and-auto.md): what a probe costs, `agx usage --logins`, `--login auto`.

### Configuration and shell

- [Config reference](guides/config-shell/config-reference.md): every key, discovery, edits by `agx login`, every validation message.
- [Environment variables](guides/config-shell/environment.md): `AGX_CONFIG_DIR`, `AGX_HOME`, `AGX_LOGIN`, `NO_COLOR`, the shell-layer variables, what agx changes for the agent.
- [Shell integration](guides/config-shell/shell-integration.md): the layer, the launch-plan protocol, aliases, `aglogin`, completion.

### Choosing

- [Choosing guides](choosing/README.md): the index.
- [Which profile, which login](choosing/which-profile-or-login.md)
- [Which token source](choosing/which-token-source.md)
- [Running and resuming](choosing/running-and-resuming.md)
- [Library or CLI](choosing/library-or-cli.md)

### Go library

- [Use agx as a Go library](guides/library/README.md): the package map, the layering, running the examples.
- [The provider model](guides/library/provider-model.md): implementing `provider.Provider` and its optional upgrades.
- [Claude Code and Codex providers](guides/library/claude-and-codex.md): usage, the probe, conversations, history moves, launch commands.
- [Config, auto-pick and session folders](guides/library/config-pick-sessions.md): `config`, `pick`, `sessions`.
- [Secrets and logins](guides/library/secrets-and-logins.md): `secret` references, the token file, `login.Resolver`.

### Examples

Runnable programs, each with its real output; [`examples/examples_test.go`](../examples/examples_test.go) runs them all in `go test ./...`. Index: [examples/README.md](../examples/README.md).

- [autopick](../examples/autopick/): `pick.Best` and `Result.Explain`.
- [config-resolve](../examples/config-resolve/): `config.Load`, `Decode`, `Resolve`, strict decoding, alias validation.
- [conversations](../examples/conversations/): Claude conversations, `EncodePath`, `HasHistory`, history moves.
- [custom-provider](../examples/custom-provider/): implementing `provider.Provider` for another CLI.
- [launch-command](../examples/launch-command/): `Launch` for Claude Code and Codex, the home-variable rule, a token login's env.
- [login-resolve](../examples/login-resolve/): `login.Resolver`, `TokenFile`, `ExpiresAt`, `Compatible`.
- [secret-refs](../examples/secret-refs/): `secret.Parse`, `System`, `ResolveAll`, a custom `Resolver`.
- [sessions-gc](../examples/sessions-gc/): `sessions.Create`, `List`, `PlanGC`, `RemoveEmpty`, `Promote`.
- [usage-fake-server](../examples/usage-fake-server/): usage and the probe against an `httptest` server, 429 and 401.

### Reference

- [Feature index](reference/features.md): every feature, the package that implements it, its guide section and its example.

## Keeping these docs right

`task docs:all` runs every docs gate: `docs:check` (docsync on the project README), `docs:snippets:check` (Go code and program output quoted in these pages must match the real source and the real example output; `task docs:snippets` regenerates them) and `docs:lint` (no hard-wrapped prose, no broken relative links or anchors, no machine-local paths, no authorship wording). The tools live in [`scripts/docs/`](../scripts/docs/).
