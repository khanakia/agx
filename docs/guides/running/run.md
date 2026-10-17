# Start an agent with `agx run`

`agx run` starts Claude Code or Codex in the current folder on one of your profiles: it picks the profile's home (history, settings, login), sets or removes the environment variables that select that home, adds the profile's own arguments, environment and secrets, and then replaces itself with the agent. Use it whenever you would type `claude` or `codex` yourself but want to choose the account by name (`-p work`), by headroom (`-p auto`), or check first what would run (`--dry-run`).

## Before you start

- agx is installed and `agx profiles` lists at least one profile. With no config file, agx discovers `~/.claude`, `~/.claude-<name>` and `~/.codex` on its own; see [Accounts](../getting-started/accounts.md).
- The agent binary (`claude` or `codex`) is on your `PATH`. agx does not install it.
- The examples on this page and the other running guides use this config file, `~/.agx/config.yaml`. It defines two Claude subscription accounts, an api-billed Claude Code profile that talks to Kimi and shares `~/.claude`, and one Codex account:

```yaml
sessions:
  root: ~/work/sessions
  promote_root: ~/work
profiles:
  - name: personal
    provider: claude
    home: ~/.claude
    default: true
  - name: work
    provider: claude
    home: ~/.claude-work
  - name: kimi
    provider: claude
    home: ~/.claude
    billing: api
    env:
      ANTHROPIC_BASE_URL: https://api.moonshot.ai/anthropic
    secrets:
      ANTHROPIC_AUTH_TOKEN: env:KIMI_API_KEY
  - name: codex
    provider: codex
    home: ~/.codex
```

```text
$ agx profiles
PROFILE    PROVIDER  HOME            ACCOUNT  STATE    LOGIN  BILLING  SOURCE
*personal  claude    ~/.claude                none     home   plan     config
work       claude    ~/.claude-work           none     home   plan     config
kimi       claude    ~/.claude                api key  home   api      config
*codex     codex     ~/.codex                 none     home   plan     config
```

The `*` marks each provider's default profile. Output on this page was captured on a test machine whose home folder is shown as `/home/you`.

## Step 1 — Run on the default profile

With no `-p`, agx launches the default profile of `--provider`, which is `claude` unless you say otherwise. The default is the profile with `default: true`, or the first profile of that provider in the config.

```sh
agx run
```

To start the default Codex profile instead, name the provider:

```sh
agx run --provider codex
```

`--provider` only matters when no profile is named or when you use `-p auto`; with `-p work` the profile already decides the provider.

## Step 2 — Pick a profile with `-p`

`-p` (long form `--profile`) takes any name from `agx profiles`, of any provider:

```sh
agx run -p work
agx run -p codex
agx run -p kimi
```

`-p auto` picks the plan-billed account with the most headroom; it has its own guide, [Auto-pick](auto-pick.md).

## Step 3 — Pass arguments to the agent

Everything after `--` goes to the agent unchanged, after the profile's own `args`:

```sh
agx run -p work -- --model sonnet -c
```

Plain words without a leading dash are passed on too, so `agx run "explain this repo"` hands the prompt to Claude Code. Anything that starts with `-` and comes before `--` is read as an agx flag.

## Step 4 — Check what would run with `--dry-run`

`--dry-run` prints the launch and starts nothing. It is accepted by `run`, `new`, `resume` and `ws`.

```sh
agx run -p work --dry-run
```

The lines mean:

| Line | Meaning |
|---|---|
| `profile:` | the profile, its provider and its home |
| `login:` | whose quota the session spends: `home` is the home's own login; see [Logins](../logins/overview.md) |
| `dir:` | the folder the agent starts in |
| `env:` | only the variables agx changed: `NAME=value` for a variable set, `-NAME (removed)` for one taken out of your environment, `NAME=<secret:scheme>` for a secret (its value is never resolved or printed in a dry run) |
| `exec:` | the exact command line |

## Step 5 — Know the default-home rule

Claude Code reads its home from `CLAUDE_CONFIG_DIR` and Codex from `CODEX_HOME`. For any home other than the default one, agx sets the variable. For the default home (`~/.claude`, `~/.codex`) agx removes the variable from the child environment, even if your shell exported it.

```sh
CLAUDE_CONFIG_DIR=~/.claude agx run --dry-run
```

Why removal and not "set it to `~/.claude`": Claude Code derives the name of its keychain entry from whether `CLAUDE_CONFIG_DIR` is set. Setting it, even to the default folder, makes Claude Code look for a different keychain entry, and the account appears logged out (`provider/env.go`, `HomeEnv`). The same rule is applied to Codex so both behave alike.

agx also removes `CLAUDE_CODE_CHILD_SESSION`. Claude Code sets it on its own helper processes, and a terminal opened from inside a Claude session inherits it; a top-level session started with it present does not save its transcript, so it could never be resumed (`internal/cli/launch.go`, `childSessionEnv`).

## Step 6 — Run an api-billed profile

A profile with `billing: api` (such as `kimi` above) is launched like any other. Its `env` values are set as written, and each `secrets` entry is resolved at launch from its scheme (`env:` reads a variable, `gopass:` reads a gopass entry) and set on the child only. It is never auto-picked and has no usage windows.

```sh
KIMI_API_KEY=your-key agx run -p kimi
```

## Try it

```text
$ agx run --dry-run
profile: personal (claude, /home/you/.claude)
login:   home
dir:     /home/you/code/app
exec:    claude

$ agx run -p work --dry-run -- --model sonnet -c
profile: work (claude, /home/you/.claude-work)
login:   home
dir:     /home/you/code/app
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude --model sonnet -c

$ agx run -p kimi --dry-run
profile: kimi (claude, /home/you/.claude)
login:   home
dir:     /home/you/code/app
env:     ANTHROPIC_AUTH_TOKEN=<secret:env>
env:     ANTHROPIC_BASE_URL=https://api.moonshot.ai/anthropic
exec:    claude

$ agx run --provider codex --dry-run
profile: codex (codex, /home/you/.codex)
login:   home
dir:     /home/you/code/app
exec:    codex

$ CLAUDE_CONFIG_DIR=~/.claude agx run --dry-run
profile: personal (claude, /home/you/.claude)
login:   home
dir:     /home/you/code/app
env:     -CLAUDE_CONFIG_DIR (removed)
exec:    claude

$ CLAUDE_CODE_CHILD_SESSION=1 agx run --dry-run
profile: personal (claude, /home/you/.claude)
login:   home
dir:     /home/you/code/app
env:     -CLAUDE_CODE_CHILD_SESSION (removed)
exec:    claude
```

## Pitfalls

- **A wrapper that exports `CLAUDE_CONFIG_DIR=~/.claude` logs you out.** If you write your own script around Claude Code, leave the variable unset for the default home, exactly as agx does (`provider/env.go`, `HomeEnv`; tested in `provider/provider_test.go`).
- **Agent flags must come after `--`.** `agx run --model sonnet` stops with `agx: unknown flag: --model` and exit code 2, because cobra reads `--model` as an agx flag. Write `agx run -- --model sonnet`.
- **`--dry-run` does not resolve secrets.** A broken `gopass:` reference or an unset `env:` variable passes the dry run and fails only on the real launch (`internal/cli/launch.go`: secrets are resolved only when `dryRun` is false).
- **The agent must be on `PATH`.** Otherwise the launch fails with `agx: claude not found on PATH: exec: "claude": executable file not found in $PATH`, exit 1 (`internal/cli/exec_unix.go`).
- **agx replaces itself with the agent.** On macOS and Linux it calls `exec`, so no agx process stays behind, and the exit status, signals and job control are the agent's own (`internal/cli/exec_unix.go`, `execReplace`).
- **An unknown profile is a usage error.** `agx run -p nope` prints ``agx: unknown profile "nope" (see `agx profiles`)`` and exits 2; an unknown `--provider` prints `agx: unknown provider "gemini"` and exits 2 even when `-p` names a valid profile (`internal/cli/launch.go`, `resolveProfile`).
- **`auto` is not a profile name.** It is reserved for auto-pick, and a config that names a profile `auto` is rejected (`config/config.go`, `resolveProfile`).

## Related

- [Auto-pick the account with the most headroom](auto-pick.md)
- [Session folders: `agx new` and `agx sessions`](new-and-sessions.md)
- [Resume a conversation](resume.md)
- [Logins](../logins/overview.md) for `--login`, which picks whose quota a launch spends
- [Configuration](../config-shell/config-reference.md) for every profile key (`args`, `args_replace`, `env`, `secrets`, `billing`)
- [README: Start an agent here](../../../README.md#start-an-agent-here-agx-run)
