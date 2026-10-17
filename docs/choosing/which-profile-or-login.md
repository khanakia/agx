# Which profile, which login

Every launch answers two separate questions: which home the agent runs in (the profile: its history, settings and memory) and whose quota it spends (the login: the home's own, a named token login, or an API key). This page helps you choose each one, and then choose how to set it: a flag for one run, `AGX_LOGIN` for one terminal, or a default in `config.yaml`. Read it when you are about to add a second account, when a run spent the wrong quota, or when you are unsure whether you need a new home or only a new login.

## Before you start

- agx installed and at least one Claude Code home it can see (`agx profiles`); see [Getting started](../guides/getting-started/README.md).
- The two words used throughout: a **home** is a folder such as `~/.claude` or `~/.claude-work` that holds conversations and settings; a **login** is a credential. A profile is a name for one home plus how to launch it. See [Logins overview](../guides/logins/overview.md#step-1--know-the-two-words-home-and-login).
- Logins exist for Claude Code only. A Codex profile always runs on its home's own login (`provider/codex` implements no `LoginLauncher`, so `agx login add --provider codex` is refused).

## Step 1 — Decide whether you need a new home or only a new login

The question to ask is: should the conversations be kept apart, or only the bill?

| You want | Choose | Why | Guide | Example |
|---|---|---|---|---|
| A second account whose chats, memory and settings stay separate (work vs personal) | a new **home** and a profile for it | history lives in the home; a login does not move it | [Find and add accounts](../guides/getting-started/accounts.md#step-2--add-a-second-claude-code-account) | [config-resolve](../../examples/config-resolve/) |
| Keep working in this home but spend another plan's quota when this one runs low | a **token login** (`oauth-token`, from `claude setup-token`) | the home and its history stay; only `CLAUDE_CODE_OAUTH_TOKEN` changes | [Add and manage logins](../guides/logins/add-and-manage.md#step-2--add-a-stored-login) | [login-resolve](../../examples/login-resolve/) |
| Pay per token for this home, for example on a CI box | an **API-key login** (`--kind api-key`) | delivered as `ANTHROPIC_API_KEY`, never expires, has no plan windows | [Add and manage logins](../guides/logins/add-and-manage.md#step-4--pick-a-kind) | [launch-command](../../examples/launch-command/) |
| A third-party backend such as Kimi that speaks the same API | a **profile** with `billing: api` and its own `env:` / `secrets:` | the backend's key is part of the profile, so no login can be put on it | [Run an api-billed profile](../guides/running/run.md#step-6--run-an-api-billed-profile) | [config-resolve](../../examples/config-resolve/) |
| Conversations already made on one account, now wanted on another | neither: **move the history** | a login changes the bill, not where the chats are stored | [Move history](../guides/running/move-history.md) | [conversations](../../examples/conversations/) |

If X then Y:

- If you would be unhappy to see the other account's chats in `agx resume`, make a new home.
- If you only ever hit the 5-hour or weekly cap, add a token login and keep one home.
- If a profile has `billing: api`, or its own `env:` or `secrets:` sets `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN`, it cannot take a login at all (`login/login.go`, `Compatible`).

## Step 2 — Choose the login kind

| | Home login | Token login (`oauth-token`) | API-key login (`api-key`) |
|---|---|---|---|
| What it is | whatever `claude` stored in the home when you logged in | a long-lived token from `claude setup-token` | a console API key |
| Billing | the home account's plan | the token account's plan | per token |
| Env set at launch | nothing (the home's own credential is used) | `CLAUDE_CODE_OAUTH_TOKEN`; `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN` removed | `ANTHROPIC_API_KEY`; `CLAUDE_CODE_OAUTH_TOKEN` and `ANTHROPIC_AUTH_TOKEN` removed |
| Expiry | managed by Claude Code | one year from `added`, warned 30 days ahead (`provider/claude/login.go`, `SetupTokenLifetime`; `internal/cli/login.go`, `expiryWarnWindow`) | never |
| Usage in `agx usage` | free usage endpoint | one probe: a few tokens of its quota (`agx usage --logins`) | not probed: shown as billed per token |
| Candidate for `--login auto` | yes | yes | no: skipped as billed per token (`internal/cli/loginusage.go`, `probeLogin`; `pick/pick.go`, `Best`) |
| `agx plan` | yes | refused | refused |

The env switch is what makes the choice safe: a token login never leaves a second credential in the agent's environment for Claude Code to prefer (`provider/claude/login.go`, `LoginEnv`). Read it in the [Logins overview](../guides/logins/overview.md#step-4--understand-what-changes-in-the-agents-environment).

## Step 3 — Choose where to set it

agx takes the first of five places that names a login (`internal/cli/login.go`, `pickLogin` then `defaultLogin`):

| # | Place | Set it when | How | Lasts |
|---|---|---|---|---|
| 1 | `--login NAME`, `--login home` or `--login auto` | one run is different | the flag on `run`, `new`, `resume` or `ws` | one command |
| 2 | `AGX_LOGIN` | one terminal is different (say, a tmux pane for a long job) | `aglogin NAME` from the shell layer, or `export AGX_LOGIN=NAME` | the shell |
| 3 | the profile's `login:` | this profile should always spend that quota | `agx login use NAME -p PROFILE` | until changed |
| 4 | `providers.claude.login:` | every Claude profile without its own `login:` | `agx login use NAME --provider claude` | until changed |
| 5 | the home's own login | nothing set | — | — |

If X then Y:

- If it is a one-off, use the flag; nothing to undo.
- If you are about to run several commands on another quota in one terminal, use `aglogin NAME`, and `aglogin` with no argument when done ([Choosing a login: switch one terminal](../guides/logins/choosing-a-login.md#step-5--switch-one-terminal)).
- If a profile should never use its home's login, set it on the profile. `login: home` on a profile shields it from a provider default.
- If you want agx to choose per run, use `--login auto`. It is accepted only as the flag; `AGX_LOGIN=auto`, `login: auto` and `agx login use auto` are refused because every auto run spends a probe on each login (`config/logins.go`, `ErrAutoLogin`).

## Step 4 — Choose `-p auto` or `--login auto`

Both use the same policy (lowest highest-window percent wins, ties in config order: `pick/pick.go`, `Best`), but they choose different things.

| | `-p auto` | `--login auto` |
|---|---|---|
| Chooses | the profile (the home) | the login for the profile you named (or the default one) |
| History | follows the chosen home, so you may land in an account without this project's chats | stays: same home, same chats |
| Cost | free usage endpoint per home | free endpoint for the home's login, plus one probe per fitting token login (`internal/cli/loginusage.go`, `autoLogin`) |
| Ties | config order | the home's own login first, then config order |
| On `resume` | not accepted: resume already uses the conversation's home | accepted |
| Guide | [Auto-pick](../guides/running/auto-pick.md) | [Login usage and `--login auto`](../guides/logins/usage-and-auto.md#step-5--let-agx-pick-the-login-for-a-run) |
| Example | [autopick](../../examples/autopick/) | [autopick](../../examples/autopick/) (same `pick.Best`) |

If your homes have separate history you care about, prefer `--login auto`: it keeps the project's conversations where they are and still spends the emptiest quota.

## Try it

A throwaway home with profiles `personal` and `work` (two Claude homes) and `kimi` (api-billed, sharing `~/.claude`), logins `personal` (stored) and `team` (cmd). The home folder is shown as `/home/you`. Each `--dry-run` prints the decision without launching:

```text
$ agx run -p work --dry-run
profile: work (claude, /home/you/.claude-work)
login:   home
dir:     /home/you
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude
$ agx run -p work --login personal --dry-run
profile: work (claude, /home/you/.claude-work)
login:   personal (--login)
dir:     /home/you
env:     CLAUDE_CODE_OAUTH_TOKEN=<token:stored>
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude
$ AGX_LOGIN=team agx run -p work --dry-run
profile: work (claude, /home/you/.claude-work)
login:   team (AGX_LOGIN)
dir:     /home/you
env:     CLAUDE_CODE_OAUTH_TOKEN=<token:cmd>
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude
$ AGX_LOGIN=team agx run -p work --login home --dry-run
profile: work (claude, /home/you/.claude-work)
login:   home (--login)
dir:     /home/you
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude
$ AGX_LOGIN=team agx run -p kimi --dry-run
agx: AGX_LOGIN=team skipped for profile kimi: login: profile already has its own credential: profile kimi is billed per token (billing: api)
profile: kimi (claude, /home/you/.claude)
login:   home
dir:     /home/you
env:     ANTHROPIC_AUTH_TOKEN=<secret:env>
env:     ANTHROPIC_BASE_URL=https://api.moonshot.ai/anthropic
exec:    claude
$ agx run -p kimi --login personal --dry-run
agx: login: profile already has its own credential: profile kimi is billed per token (billing: api)
```

The last command exits 2: a flag that does not fit is an error, while an `AGX_LOGIN` that does not fit is skipped with a note.

## Pitfalls

- **A login does not move history.** `agx run -p work --login personal` still runs in `~/.claude-work` and sees only its chats. To continue a chat on another account's home, [move it](../guides/running/move-history.md).
- **`resume -p` is not a login switch.** It starts the conversation in another profile's home, where the transcript usually does not exist (`internal/cli/launch.go`, `resumeConversation`). Use `agx resume --login NAME` to keep the home and change the quota.
- **A typo in `AGX_LOGIN` stops every launch**, even for profiles that would skip it: an unknown name is exit 2, only a known login that does not fit is skipped ([Choosing a login: pitfalls](../guides/logins/choosing-a-login.md#pitfalls)).
- **An expired stored login is refused, not silently replaced** by the home's login, so you notice before spending the wrong quota; rotate it with `agx login add NAME --replace` ([Renew before expiry](../guides/logins/add-and-manage.md#step-8--renew-before-expiry)).
- **Setting a default on a config with no profiles writes the discovered ones into it**, and from then on discovery for that provider is off ([Config reference: discovery](../guides/config-shell/config-reference.md#step-3--understand-discovery)).

## Related

- [Which token source](which-token-source.md): stored, `--cmd` or `--env`
- [Running and resuming](running-and-resuming.md): `run`, `new`, `resume`, `ws` and the session commands
- [Choosing a login](../guides/logins/choosing-a-login.md): the precedence in detail
- [Login usage and `--login auto`](../guides/logins/usage-and-auto.md)
