# Logins: run any home on any account's quota

A Claude home (`~/.claude`, `~/.claude-work`, …) holds two things at once: a conversation history and the login that pays for it. A **login** in agx separates the two. It is a named, long-lived credential (a `claude setup-token` token or an API key) that agx hands to Claude Code when it launches, so the work home can keep its history, settings and `resume` list while a run spends the personal account's quota. You want logins when one account runs out mid-week and you would rather borrow another account's headroom than move a project's history, or when a CI box should run on a token without anyone logging in interactively.

## Before you start

- agx installed and at least one Claude home that agx sees (`agx profiles` lists it). With no config file, `~/.claude` becomes profile `personal` and every `~/.claude-<name>` becomes profile `<name>`; see [Config reference](../config-shell/config-reference.md#step-3--understand-discovery).
- A long-lived token. For plan quota run `claude setup-token` and copy what it prints (or let `agx login add --setup` run it for you). For per-token billing use a console API key.
- Logins exist only for Claude Code today. Codex has none; see [Pitfalls](#pitfalls).

## Step 1 — Know the two words: home and login

A **home** is a directory: where the vendor CLI keeps history, settings and its own interactive login. A **profile** points at one home. A **login** is a credential with a name; it lives in agx's config, not in any home. When a run uses a login, agx sets that credential in the agent's environment and leaves the home untouched, so history and resume stay where they were.

```text
profile work ──► home ~/.claude-work   (history, settings, resume list)
             └─► login personal        (whose quota this run spends)
```

Without a login, a run uses "the home's own login": whatever the vendor CLI stored in that home when you logged in. agx calls that choice `home`.

## Step 2 — Add a login

The token never goes on the command line (argv is visible in `ps` and shell history). Pipe it in, paste it at a hidden prompt, or point agx at a command or variable that holds it:

```sh
pbpaste | agx login add personal --stdin --account you@example.com
```

agx writes the raw value to `~/.agx/tokens.yaml` (mode 0600) and only the word `stored` to `~/.agx/config.yaml`. Every way of adding a login is in [Add and manage logins](add-and-manage.md).

## Step 3 — Run on it

```sh
agx run -p work --login personal
```

The work home and its history, the personal account's quota. agx prints `agx: work · login personal` on stderr before it starts the agent; transcripts do not record which login paid, so that line is the only record. To make it the default instead of a per-run flag, see [Choosing a login](choosing-a-login.md).

## Step 4 — Understand what changes in the agent's environment

agx changes three Claude Code credential variables and nothing else. The other two are removed because either would otherwise take precedence and bill the wrong way (`provider/claude/login.go`, `LoginEnv`):

| Login kind | Set | Removed |
|---|---|---|
| `oauth-token` (default, plan quota) | `CLAUDE_CODE_OAUTH_TOKEN` | `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` |
| `api-key` (billed per token) | `ANTHROPIC_API_KEY` | `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_AUTH_TOKEN` |

`CLAUDE_CONFIG_DIR` is handled exactly as without a login: set for a non-default home, removed for `~/.claude`. Check any run with `--dry-run`, which shows the variables with the token replaced by its source:

```sh
agx run -p work --login personal --dry-run
```

## Try it

A throwaway home with two Claude homes and one stored login (the home directory is shown as `/home/you`):

```text
$ printf 'sk-ant-oat01-FAKE-personal-0000000000\n' | agx login add personal --stdin --account me@example.com
added login personal (claude, oauth-token, stored)
agx: tokens.yaml holds raw tokens: never sync, share or commit it (~/.agx/tokens.yaml)
login personal: resolved ok (not probed)
see its usage with `agx login check personal` (one probe: a few tokens of its quota)

$ agx run -p work --login personal --dry-run
profile: work (claude, /home/you/.claude-work)
login:   personal (--login)
dir:     /home/you
env:     CLAUDE_CODE_OAUTH_TOKEN=<token:stored>
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude
```

## Pitfalls

- **Codex has no logins.** `agx login add NAME --provider codex` is refused: a ChatGPT-plan login exists only as refreshable tokens in `<home>/auth.json`, and copying them risks logging the CLI out (`config/logins.go`, `NoLoginsReason`). The real message: `agx: codex has no token logins yet: a ChatGPT-plan login exists only as refreshable tokens in <home>/auth.json (copying it risks logging the CLI out), and the API-key variable interactive Codex honours is unverified (spec §12 V5)`.
- **The agent can read the token.** It is in the agent's environment, so every tool, hook or MCP server the agent starts can read it too. Use a login only with projects you would trust with that account.
- **agx never falls back to the home's login on its own.** A missing, empty or multi-line token, a failing `--cmd`, or an unset `--env` variable stops the launch with the reason (`internal/cli/login.go`, `resolveLoginValue`; `login/login.go`, `Resolve`). You asked for a specific account, so agx does not quietly pick another.
- **Removing a login does not revoke the token.** `agx login rm` says so every time (`revokeHint` in `internal/cli/login.go`). Revoke it in your claude.ai account settings.
- **A login cannot ride on a profile that already has a credential.** An api-billed profile, or one whose `env:` / `secrets:` sets any of the three variables above, is refused (`login/login.go`, `Compatible`); see [Choosing a login](choosing-a-login.md#pitfalls).

## Related

- [Add and manage logins](add-and-manage.md)
- [Choosing a login](choosing-a-login.md)
- [Login usage and `--login auto`](usage-and-auto.md)
- [Config reference: `logins`](../config-shell/config-reference.md#logins)
