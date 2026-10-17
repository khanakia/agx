# Choosing a login

Every launch (`run`, `new`, `resume`, `ws`) runs on exactly one login: a named login or the home's own. agx decides which from five places, in a fixed order, so you can set a default once for a profile or a whole provider, override it for one terminal, and override that again for one run. Use this page when a profile should always spend another account's quota, when a terminal should temporarily, or when a run used a login you did not expect.

## Before you start

- At least one login (`agx login ls`); see [Add and manage logins](add-and-manage.md).
- For `aglogin`, the shell layer loaded: `eval "$(agx shell-init zsh)"`; see [Shell integration](../config-shell/shell-integration.md).

## Step 1 — Learn the precedence

The first match wins (`internal/cli/login.go`, `pickLogin` and `defaultLogin`):

| # | Where | Set with | If the login does not fit the profile |
|---|---|---|---|
| 1 | `--login NAME`, `--login home` or `--login auto` on the command | the flag | error, exit 2 |
| 2 | `AGX_LOGIN` in the environment | `aglogin NAME` or `export AGX_LOGIN=NAME` | skipped with a one-line note; steps 3 to 5 apply |
| 3 | the profile's `login:` in config.yaml | `agx login use NAME -p PROFILE` | config refused at load |
| 4 | `providers.<id>.login:` in config.yaml | `agx login use NAME --provider ID` | skipped silently |
| 5 | the home's own login | nothing | — |

`home` is a valid value at steps 1 to 4 and means "stop here and use the home's own login". A profile with `login: home` therefore ignores a provider default. `auto` is accepted only at step 1; see [Login usage and `--login auto`](usage-and-auto.md).

"Does not fit" means: a login of another provider, a profile with `billing: api`, or a profile whose own `env:` or `secrets:` sets a credential variable the login would set or remove (`login/login.go`, `Compatible`). That is why an exported Claude login never breaks a Codex or Kimi run.

## Step 2 — Use a login for one run

```sh
agx run -p work --login personal
```

Every alias passes extra arguments through, so `clw --login personal` works too. To ignore every default for one run and use the home's own login:

```sh
agx run -p work --login home
```

## Step 3 — Make it the default for a profile

```sh
agx login use personal -p work
```

This writes `login: personal` under profile `work` in config.yaml and prints the key before and after. Undo it with:

```sh
agx login use home -p work
```

## Step 4 — Make it the default for a whole provider

```sh
agx login use personal --provider claude
```

Every Claude profile without its own `login:` now runs on `personal`, except profiles it does not fit (an api-billed Kimi profile, for example), which quietly keep their home's login. Undo with `agx login use home --provider claude`. Give exactly one of `-p` or `--provider`.

## Step 5 — Switch one terminal

`aglogin` is a shell function from `agx shell-init`, because a child process cannot change its parent shell's environment:

```sh
aglogin personal
```

It first asks the binary whether `personal` exists (the hidden `agx login exists`), so a typo fails now rather than on the next run, then exports `AGX_LOGIN=personal`. `aglogin` with no argument unsets it.

## Step 6 — See what a run would use

```sh
agx profiles
agx run -p work --dry-run
```

`agx profiles` has a `LOGIN` column naming each profile's login and the step it came from: `personal (profile)`, `personal (provider)`, `viacmd (AGX_LOGIN)`, `home`. `--dry-run` prints the same choice plus the environment, with the token shown only as `<token:stored>`, `<token:cmd>` or `<token:env>`; the token is not resolved.

## Try it

A throwaway home (`/home/you`) with logins `personal` (stored) and `viacmd` (cmd), profiles `personal` and `work`:

```text
$ agx login use personal -p work
note: wrote discovered profile personal into the config
note: wrote discovered profile work into the config
~/.agx/config.yaml: profiles[work].login
- login: (unset)
+ login: personal

$ agx profiles
PROFILE    PROVIDER  HOME            ACCOUNT  STATE  LOGIN               BILLING  SOURCE
*personal  claude    ~/.claude                none   home                plan     config
work       claude    ~/.claude-work           none   personal (profile)  plan     config

$ AGX_LOGIN=viacmd agx run -p work --dry-run
profile: work (claude, /home/you/.claude-work)
login:   viacmd (AGX_LOGIN)
dir:     /home/you
env:     CLAUDE_CODE_OAUTH_TOKEN=<token:cmd>
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude

$ agx run -p work --login home --dry-run
profile: work (claude, /home/you/.claude-work)
login:   home (--login)
dir:     /home/you
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude
```

An exported login that does not fit a per-token profile is skipped, not fatal; the same login named with `--login` is an error:

```text
$ AGX_LOGIN=personal agx run -p kimi --dry-run
agx: AGX_LOGIN=personal skipped for profile kimi: login: profile already has its own credential: profile kimi is billed per token (billing: api)
profile: kimi (claude, /home/you/.claude)
login:   home (profile)
dir:     /home/you
env:     ANTHROPIC_AUTH_TOKEN=<secret:env>
env:     ANTHROPIC_BASE_URL=https://api.moonshot.ai/anthropic
exec:    claude

$ agx run -p kimi --login personal --dry-run
agx: login: profile already has its own credential: profile kimi is billed per token (billing: api)
```

`aglogin` in bash:

```text
$ aglogin nope
agx: unknown login "nope" (known: ci, home)
$ aglogin ci
agx: AGX_LOGIN=ci in this terminal (aglogin with no argument undoes it)
$ aglogin
agx: AGX_LOGIN unset — profile and provider defaults apply
```

## Pitfalls

- **`agx login use` on a config with no profiles writes the discovered ones into it.** That is the `note: wrote discovered profile …` line above. Once a provider has any configured profile, discovery for it is off, so a `~/.claude-<name>` home you create later does not appear until you add it to `profiles:` (`config/config.go`, `Resolve`). See [Config reference: discovery](../config-shell/config-reference.md#step-3--understand-discovery).
- **A bad `AGX_LOGIN` stops every launch.** `AGX_LOGIN=nope` gives `agx: AGX_LOGIN=nope: unknown login (see `agx login ls`; `aglogin` with no argument unsets it)` with exit 2, even for profiles that would never use it. Only a login that exists but does not fit is skipped.
- **`auto` is refused everywhere except the flag.** `AGX_LOGIN=auto`, `login: auto` in config.yaml, `agx login use auto` and `aglogin auto` all fail with `"auto" is accepted only as --login on run, new, resume and ws (it probes every login, spending a few tokens of each account's quota)` (`config/logins.go`, `ErrAutoLogin`).
- **A profile that sets its own credential cannot take a login.** `env: {ANTHROPIC_API_KEY: …}` plus `login: p` is refused at load: `profiles[0] (a): login: profile already has its own credential: profile a sets ANTHROPIC_API_KEY in env`.
- **`agx new --login TYPO` still creates the session folder.** The folder is created before the login is checked, and with the shell layer the error appears only after the shell has changed into that folder (`internal/cli/launch.go`, `newNewCmd` creates the folder and writes the plan with the unchecked `--login`). `agx sessions gc --yes` removes the empty folder afterwards.
- **`agx plan` takes only `--login home`.** `agx plan --login personal` is refused: the plan comes from the profile endpoint, which refuses setup-tokens.

## Related

- [Logins overview](overview.md)
- [Add and manage logins](add-and-manage.md)
- [Login usage and `--login auto`](usage-and-auto.md)
- [Environment variables: `AGX_LOGIN`](../config-shell/environment.md#step-2--pick-a-login-for-a-terminal-agx_login)
- [Shell integration: `aglogin`](../config-shell/shell-integration.md#step-4--switch-logins-per-terminal-with-aglogin)
