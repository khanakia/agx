# Environment variables

agx reads a handful of environment variables: two that move its config folder, one that picks a login for a whole terminal, two that the shell layer uses to talk to the binary, and `NO_COLOR`. It also sets or removes a few credential and home variables in the environment of the agent it launches. Use this page when you keep agx's files somewhere other than `~/.agx`, script agx in CI, or wonder why a variable you exported did or did not reach the agent.

## Before you start

- `agx doctor` shows which rule chose the config folder, which is the quickest way to confirm any of the folder variables below.
- Variables set to an empty string count as unset for `AGX_CONFIG_DIR`, `AGX_HOME` and `AGX_LOGIN`.

## Step 1 — Move the config folder: `AGX_CONFIG_DIR` and `AGX_HOME`

agx keeps `config.yaml` and `tokens.yaml` in one folder. It is chosen by the first rule that applies (voltkit `appdir`, with agx's prefix `AGX` and dot-dir `.agx`):

| Order | Rule | Folder | `agx doctor` says |
|---|---|---|---|
| 1 | `AGX_CONFIG_DIR` is set | `$AGX_CONFIG_DIR` | `decided by env-category` |
| 2 | `AGX_HOME` is set | `$AGX_HOME/config` | `decided by env-home` |
| 3 | otherwise | `~/.agx` | `decided by home-dotfile` |

`tokens.yaml` always sits beside `config.yaml`, so both variables move it too. A config folder for one CI job:

```sh
export AGX_CONFIG_DIR="$HOME/ci/agx"
agx doctor
```

## Step 2 — Pick a login for a terminal: `AGX_LOGIN`

`AGX_LOGIN=NAME` makes every launch in that environment run on login `NAME`, unless `--login` is given (step 2 of [the login precedence](../logins/choosing-a-login.md#step-1--learn-the-precedence)). `home` means the home's own login; `auto` is refused. Set it with the `aglogin` shell function, which checks the name first, or export it yourself:

```sh
export AGX_LOGIN=personal
agx run -p work --dry-run
```

A login that exists but does not fit a profile (another provider, an api-billed profile) is skipped for that profile with a one-line note; an unknown name stops every launch.

## Step 3 — Colour: `NO_COLOR`

When `NO_COLOR` is present in the environment (any value, even empty), `agx usage` prints no ANSI colour unless you pass `--color always` (`internal/cli/usage.go`, `wantColor`). Without it, colour is on only when stdout is a terminal.

```sh
NO_COLOR=1 agx usage
```

## Step 4 — Know the two variables the shell layer uses

You do not set these yourself; the generated shell code does ([Shell integration](shell-integration.md)).

| Variable | Set by | Read by | Purpose |
|---|---|---|---|
| `AGX_SHELL_INIT` | `eval "$(agx shell-init zsh)"` exports `zsh` or `bash` | `agx doctor` | Reports `shell layer active (zsh)`; unset, doctor suggests the `eval` line. |
| `AGX_PLAN_FILE` | the `agx` shell function, for `new`, `resume` and `ws` only | the binary | When set, those commands write a launch plan to this path instead of launching, so the shell can `cd` first. |

## Step 5 — Know what agx changes for the agent

The agent inherits your environment, with these changes:

| Variable | When | What agx does |
|---|---|---|
| `CLAUDE_CONFIG_DIR` | Claude profile on a non-default home | set to the home |
| `CLAUDE_CONFIG_DIR` | Claude profile on `~/.claude` | removed, even if you exported it |
| `CODEX_HOME` | Codex profile on a non-default home / on `~/.codex` | set / removed |
| `CLAUDE_CODE_OAUTH_TOKEN` | run on an `oauth-token` login | set to the token; `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN` removed |
| `ANTHROPIC_API_KEY` | run on an `api-key` login | set to the key; `CLAUDE_CODE_OAUTH_TOKEN` and `ANTHROPIC_AUTH_TOKEN` removed |
| anything in a profile's `env:` / `secrets:` | that profile | set (secrets resolved at launch) |

The default home is launched with the variable absent, not set to `~/.claude`: setting it, even to the default path, makes Claude Code look for its login under a different keychain name and the account appears logged out. `--dry-run` shows every change, with tokens and secrets replaced by their source.

## Try it

Throwaway home (`/home/you`) with an empty config file in each candidate folder:

```text
$ AGX_CONFIG_DIR=/home/you/alt agx doctor | head -1
✓ config             ~/alt/config.yaml (decided by env-category)

$ AGX_HOME=/home/you/agxhome agx doctor | head -1
✓ config             ~/agxhome/config/config.yaml (decided by env-home)

$ AGX_HOME=/home/you/agxhome AGX_CONFIG_DIR=/home/you/alt agx doctor | head -1
✓ config             ~/alt/config.yaml (decided by env-category)

$ AGX_CONFIG_DIR= agx doctor | head -1
✓ config             ~/.agx/config.yaml (decided by home-dotfile)
```

`AGX_CONFIG_DIR` wins over `AGX_HOME`, and an empty value is ignored. A login per terminal:

```text
$ AGX_LOGIN=viacmd agx run -p work --dry-run
profile: work (claude, /home/you/.claude-work)
login:   viacmd (AGX_LOGIN)
dir:     /home/you
env:     CLAUDE_CODE_OAUTH_TOKEN=<token:cmd>
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude

$ AGX_LOGIN=auto agx run -p work --dry-run
agx: AGX_LOGIN: "auto" is accepted only as --login on run, new, resume and ws (it probes every login, spending a few tokens of each account's quota)

$ AGX_LOGIN=nope agx run -p work --dry-run
agx: AGX_LOGIN=nope: unknown login (see `agx login ls`; `aglogin` with no argument unsets it)
```

## Pitfalls

- **A relative `AGX_CONFIG_DIR` follows your current directory.** Run from `/tmp`, `AGX_CONFIG_DIR=alt` reads `/tmp/alt/config.yaml` (seen in a real run: `no file at /tmp/alt/config.yaml`). Always export an absolute path.
- **Moving the folder moves `tokens.yaml`.** A login stored under `~/.agx` is `stored (missing)` when you run with `AGX_HOME` or `AGX_CONFIG_DIR` pointing elsewhere; `AGX_HOME=/home/you/agxhome agx login ls` in that throwaway home printed `no logins`, because it read a different config.
- **A stale `AGX_LOGIN` breaks every launch in that shell.** An unknown name is an error, not a skip. `aglogin` with no argument, or `unset AGX_LOGIN`, clears it.
- **`AGX_PLAN_FILE` in your own environment changes `new`, `resume` and `ws`.** With it set, they write a plan and exit 0 without launching anything (`internal/cli/launch.go`, `launchOrPlan`). Never export it in a profile or CI job.
- **Exporting `CLAUDE_CONFIG_DIR` does not pick a home for agx.** agx sets or removes it per profile; choose the home with `-p` instead.

## Related

- [Config reference](config-reference.md)
- [Shell integration](shell-integration.md)
- [Choosing a login](../logins/choosing-a-login.md)
