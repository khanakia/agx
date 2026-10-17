# Find and add accounts

agx works on profiles: one profile is one agent (Claude Code or Codex) with one home folder, and the home's own login is the account. Without a config file, agx finds homes on disk and names a profile for each; with a config file, you list the profiles yourself. This guide explains exactly which folders are found and what they are called, then adds a second Claude Code account and a second Codex account. You want it when `agx profiles` shows fewer accounts than you have, or when a profile has a name you did not expect.

## Before you start

- agx installed ([Install agx](README.md)).
- Claude Code (`claude`) and/or Codex (`codex`) on your `PATH`, because adding an account means logging in with the agent itself. agx never logs in for you and never refreshes or writes a login.

## Step 1 — See what agx discovered

```sh
agx profiles
```

These are the discovery rules, from `provider/claude/discover.go`, `provider/codex/codex.go` and `config/config.go`:

| Folder | Becomes profile | Found when |
|---|---|---|
| `~/.claude` | `personal` (the default Claude profile) | the folder holds any of `settings.json`, `projects`, `.credentials.json`, `.claude.json` |
| `~/.claude-<name>` | `<name>` (for example `~/.claude-work` → `work`), sorted by name | the folder holds any of the same four entries |
| `~/.codex` | `codex` (the default Codex profile) | the folder holds `auth.json` |

A few more rules decide the rest:

- A `~/.claude-<name>` folder without one of those four entries is ignored, so a look-alike such as `~/.claude-worktrees` never becomes an account.
- A Codex home whose `auth.json` holds only an API key (no ChatGPT login) is discovered with `BILLING api`: it has no usage windows and `-p auto` never picks it.
- The SOURCE column says `discovered` for these and `config` for profiles from `~/.agx/config.yaml`.
- `*` marks each provider's default profile, the one `agx run` uses without `-p`. For discovered Claude profiles that is the one on `~/.claude`.

## Step 2 — Add a second Claude Code account

Claude Code keeps one account per config folder, chosen by `CLAUDE_CONFIG_DIR`. Start it once on a new folder whose name begins with `.claude-` and log in there:

```sh
CLAUDE_CONFIG_DIR=~/.claude-work claude
```

Log in with the second account inside Claude Code, then quit it with `/exit`. Claude Code has now written its files into `~/.claude-work`, so the next `agx profiles` lists a `work` profile. Nothing else is needed while you have no config file. From now on start that account with `agx run -p work` rather than setting the variable by hand.

## Step 3 — Add a second Codex account

Codex keeps one account per `CODEX_HOME`, but agx discovers only `~/.codex`. Log in on a second home, then list both Codex homes in `~/.agx/config.yaml`:

```sh
CODEX_HOME=~/.codex-work codex login
```

```yaml
# ~/.agx/config.yaml
profiles:
  - name: codex
    provider: codex
    home: ~/.codex
    default: true
  - name: codex-work
    provider: codex
    home: ~/.codex-work
```

Both entries are needed: as soon as one Codex profile is configured, agx stops discovering Codex homes, so `~/.codex` would otherwise disappear from the list. Claude Code homes are still discovered, because discovery is switched off per provider. If a home is logged in with an API key only, add `billing: api` to its entry (a configured profile is `billing: plan` unless you say otherwise; Try it shows `codex-work` with it).

## Step 4 — Name the Claude profiles yourself (optional)

Discovered names come from folder names. To choose your own, or to add `args`, `env` or `secrets` to a profile, list every Claude Code home you want. The order in the file is the order agx shows them and breaks ties when `-p auto` sees two accounts with the same headroom:

```yaml
# ~/.agx/config.yaml
profiles:
  - name: personal
    provider: claude
    home: ~/.claude
    default: true
  - name: work
    provider: claude
    home: ~/.claude-work
  - name: codex
    provider: codex
    home: ~/.codex
```

Every key a profile accepts (`billing`, `args`, `args_replace`, `env`, `secrets`, `login`) is described in [Config reference](../config-shell/config-reference.md).

## Try it

Two Claude Code homes and two Codex homes (`~/.codex` and `~/.codex-work` both logged in with an API key), no config file:

```text
$ agx profiles
PROFILE    PROVIDER  HOME            ACCOUNT  STATE  LOGIN  BILLING  SOURCE
*personal  claude    ~/.claude                none   home   plan     discovered
work       claude    ~/.claude-work           none   home   plan     discovered
*codex     codex     ~/.codex                 ok     home   api      discovered
```

`~/.codex-work` is not listed. With the Step 3 config file (and `billing: api` added to `codex-work`):

```text
$ agx profiles
PROFILE     PROVIDER  HOME            ACCOUNT  STATE  LOGIN  BILLING  SOURCE
*codex      codex     ~/.codex                 ok     home   plan     config
codex-work  codex     ~/.codex-work            ok     home   api      config
*personal   claude    ~/.claude                none   home   plan     discovered
work        claude    ~/.claude-work           none   home   plan     discovered
```

Configured profiles come first, then the discovered ones of the other provider. Note that `codex` now shows `plan`, because the config entry has no `billing:` and the default is `plan`.

A logged-in machine with three Claude Code homes and a Codex API-key home, where `~/.claude` was never logged in and `~/.claude-work` holds an expired login:

```text
$ agx profiles
PROFILE    PROVIDER  HOME            ACCOUNT                       STATE              LOGIN  BILLING  SOURCE
*personal  claude    ~/.claude                                     none               home   plan     discovered
side       claude    ~/.claude-side  you@side.example · Pro        ok (1247 d 14 hr)  home   plan     discovered
work       claude    ~/.claude-work  you@work.example · Max (20x)  expired            home   plan     discovered
*codex     codex     ~/.codex                                      ok                 home   api      discovered
```

## Pitfalls

- **Configuring one profile hides the other discovered homes of that provider.** This is on purpose: "when a provider has at least one configured profile, its discovered profiles are NOT added — the config is then the complete, intentional list for that provider" (`config/config.go`, package comment and `Resolve`). List every home of that provider.
- **A configured profile does not inherit the discovered billing.** Discovery marks an API-key-only Codex home `api`; a config entry is `plan` unless it says `billing: api` (seen in Try it; `resolveProfile` in `config/config.go`). `agx usage` still prints "billed per token" for it, because the Codex provider notices the key-only `auth.json` (`provider/codex/codex.go`, `Usage`), but `agx plan` includes every `plan` profile and fails on that one: `codex: billed per token: no usage windows`, exit 1.
- **Never set `CLAUDE_CONFIG_DIR=~/.claude`.** For the default home the variable must be absent: set even to `~/.claude`, Claude Code switches to a different Keychain entry and the account looks logged out. agx removes the variable when it launches the default home (`provider/env.go`, `HomeEnv`); do the same in your own scripts. The same holds for `CODEX_HOME` and `~/.codex`.
- **Name clashes are renamed, not dropped.** A discovered profile whose name a configured profile already uses gets the provider as a prefix: with a configured Codex profile called `personal`, `~/.claude` shows up as `claude-personal` (`config/config.go`, `Resolve`).
- **A typo in the config stops every command.** Unknown keys are an error, so `defualt: true` fails with `field defualt not found in type config.ProfileSpec` and exit 2 (`config.Decode`). Run `agx doctor` after editing the file.
- **Two profiles may share one home.** For example an API-billed profile on `~/.claude` that points Claude Code at another backend. `agx usage` then shows that home once, not once per profile (`config.Homes`); see [Usage, plans and profiles](../usage/usage-and-plans.md).

## Related

- [Install agx and run it the first time](README.md)
- [Usage, plans and profiles](../usage/usage-and-plans.md): what the STATE and LOGIN columns mean.
- [Config reference](../config-shell/config-reference.md): every profile key.
- [Environment variables](../config-shell/environment.md): `AGX_CONFIG_DIR` and `AGX_HOME`.
- [Logins](../logins/overview.md): spend one account's quota from another account's home.
