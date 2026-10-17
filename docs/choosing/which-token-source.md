# Which token source

A named login gets its token from exactly one of three sources: `stored` (agx keeps it in `~/.agx/tokens.yaml`), `cmd` (a shell command prints it at each launch, typically a password manager) or `env` (another environment variable already holds it). This page helps you choose one, shows what each costs in safety and convenience, and separates logins from the profile `secrets:` key, which looks similar but solves a different problem. Read it before `agx login add`, or when moving a token out of a file and into a password manager.

## Before you start

- You have decided you want a login rather than a new home; see [Which profile, which login](which-profile-or-login.md).
- A token: `claude setup-token` for plan quota, or a console API key with `--kind api-key`. Only fake values appear on this page.
- The token never goes on the command line: there is no `--token VALUE` flag, because argv shows in `ps` and shell history ([Add and manage logins](../guides/logins/add-and-manage.md#step-1--choose-where-the-token-comes-from)).

## Step 1 — Compare the three sources

| | `stored` | `cmd` | `env` |
|---|---|---|---|
| Add with | `agx login add NAME` (hidden prompt), `--stdin`, or `--setup` | `agx login add NAME --cmd "COMMAND"` | `agx login add NAME --env VAR` |
| config.yaml key | `token: stored` | `token_cmd: COMMAND` | `token_env: VAR` |
| Where the value lives | `~/.agx/tokens.yaml`, mode 0600 in a 0700 folder | your password manager (or any program) | the environment of the shell that runs agx |
| Read when | at each launch, from the file | at each launch: `sh -c COMMAND`, no stdin, your home as working folder, 10 s limit (`login/login.go`, `cmdTimeout`) | at each launch |
| Works offline / headless | yes | only if the command does (a store that prompts for a passphrase times out) | yes, if the variable is exported |
| Expiry tracking | exact: counted from `added`; past it the login is refused | approximate (`~` in `agx login ls`); past it agx warns and runs, since you may have rotated the token in your store (`internal/cli/login.go`, `resolveLoginValue`) | approximate, as `cmd` |
| Rotate | `agx login add NAME --stdin --replace` | update the secret in your store; agx needs nothing | change the variable |
| Leaves a raw token on disk | yes, in one private file | no | no (but every child process inherits it) |
| Guide | [Add a stored login](../guides/logins/add-and-manage.md#step-2--add-a-stored-login) | [Add a login that stores nothing](../guides/logins/add-and-manage.md#step-3--add-a-login-that-stores-nothing) | [Add a login that stores nothing](../guides/logins/add-and-manage.md#step-3--add-a-login-that-stores-nothing) |
| Example | [login-resolve](../../examples/login-resolve/) (`TokenFile`, `source=stored`) | [login-resolve](../../examples/login-resolve/) (`RunCmd`, `source=cmd`) | [login-resolve](../../examples/login-resolve/) (`LookupEnv`, `source=env`) |

Whatever the source, the value is trimmed and must be exactly one non-empty line (`login/login.go`, `Clean`), and errors name the login and the source, never the value.

## Step 2 — Pick by situation

| Situation | Source | Reason |
|---|---|---|
| A laptop, one person, no password manager | `stored` | simplest; the file is checked for permissions on every read |
| You already keep tokens in gopass, 1Password CLI, `pass` or the macOS Keychain | `cmd` | the vault stays the only copy; rotation happens in one place |
| CI, a container or a systemd unit that injects secrets as variables | `env` | nothing to write to disk; the runner already owns the secret |
| The machine syncs or backs up `~` to a shared place, or `~` is a git work tree | `cmd` or `env` | `tokens.yaml` is refused when readable by others and flagged by `agx doctor` inside a git work tree |
| A token you must be told about before it lapses | `stored` | exact expiry, warned 30 days ahead, refused when past |

If X then Y:

- If your token command needs an unlock prompt, unlock the store first (or use an agent such as `gpg-agent`); the command has no stdin and is killed after 10 seconds.
- If the command can print debug output, keep it on stderr: a failing command's stderr is shown with any line of its stdout masked as `<redacted>` (`login/login.go`, `redact`), but stdout must be the token alone.
- If you export the variable for an `env` login in your shell rc, every program you start can read it; prefer `cmd` on a shared machine.

## Step 3 — Do not confuse a login with a profile secret

Both can call `gopass` or read an environment variable, but they answer different questions:

| | Login with `token_cmd` / `token_env` | Profile `secrets:` (`gopass:<path>` or `env:<NAME>`) |
|---|---|---|
| Purpose | spend another account's quota from any home | supply a profile's own credential, such as a third-party backend's key |
| Applies to | every profile it fits, chosen by `--login`, `AGX_LOGIN` or a default | one profile, always |
| Sets | `CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`, and removes the other credential variables | exactly the variable you name |
| Shown by | `agx login ls`, `agx usage --logins` | `agx doctor` (the reference parses, gopass on `PATH`) |
| Guide | [Logins overview](../guides/logins/overview.md) | [Config reference: profiles](../guides/config-shell/config-reference.md#profiles) |
| Example | [login-resolve](../../examples/login-resolve/) | [secret-refs](../../examples/secret-refs/) |

A profile whose `secrets:` or `env:` sets any variable a login would set or remove cannot take a login at all; the config is refused at load (`login/login.go`, `Compatible`).

## Try it

A throwaway home, one login per source plus an API key. The `env` login fails its check until the variable is set:

```text
$ printf 'sk-ant-oat01-FAKE-personal-0000000000\n' | agx login add personal --stdin --account me@example.com
added login personal (claude, oauth-token, stored)
agx: tokens.yaml holds raw tokens: never sync, share or commit it (~/.agx/tokens.yaml)
login personal: resolved ok (not probed)
see its usage with `agx login check personal` (one probe: a few tokens of its quota)
$ agx login add team --cmd 'printf sk-ant-oat01-FAKE-team-1111111111'
added login team (claude, oauth-token, cmd)
login team: resolved ok (not probed)
see its usage with `agx login check team` (one probe: a few tokens of its quota)
$ agx login add ci --env CI_CLAUDE_TOKEN
added login ci (claude, oauth-token, env)
login ci: failed: login ci: env CI_CLAUDE_TOKEN is not set
$ printf 'sk-ant-api03-FAKE-billing-2222222222\n' | agx login add billing --stdin --kind api-key
added login billing (claude, api-key, stored)
agx: tokens.yaml holds raw tokens: never sync, share or commit it (~/.agx/tokens.yaml)
login billing: resolved ok (not probed)
see its usage with `agx login check billing` (one probe: a few tokens of its quota)
$ agx login ls
NAME      PROVIDER  KIND         SOURCE  ACCOUNT         ADDED       EXPIRES      USED-BY
personal  claude    oauth-token  stored  me@example.com  2026-10-17  2027-10-17   —
team      claude    oauth-token  cmd     —               2026-10-17  ~2027-10-17  —
ci        claude    oauth-token  env     —               2026-10-17  ~2027-10-17  —
billing   claude    api-key      stored  —               2026-10-17  never        —
$ CI_CLAUDE_TOKEN=sk-ant-oat01-FAKE-ci-3333333333 agx login check ci --no-probe
login ci: resolved ok (not probed)
```

`agx login add ci --env …` exits 1 because its first resolve failed, but the login is added; the `~` in `EXPIRES` marks the approximate dates of `cmd` and `env` logins.

## Pitfalls

- **A raw token in config.yaml is refused.** `token:` accepts only `stored`; anything else stops every command with `a raw token does not belong in config.yaml` and the `agx login add NAME --stdin` that moves it (`config/logins.go`).
- **Exactly one source per login.** A config entry with two of `token`, `token_cmd`, `token_env` fails with `set exactly one of token: stored, token_cmd, token_env` (`config/logins.go`), and `agx login add` refuses `--cmd` with `--env`.
- **A readable `tokens.yaml` blocks every stored login**, not only one: fix it with the `chmod 600` that the error prints ([Keep tokens.yaml private](../guides/logins/add-and-manage.md#step-7--keep-tokensyaml-private)).
- **Moving the config folder moves `tokens.yaml`.** With `AGX_HOME` or `AGX_CONFIG_DIR` set, stored logins under `~/.agx` show as `stored (missing)` ([Environment variables](../guides/config-shell/environment.md#pitfalls)).
- **Codex has no logins**, whatever the source: `agx login add --provider codex` is refused, because a ChatGPT-plan login exists only as refreshable tokens in the home.

## Related

- [Which profile, which login](which-profile-or-login.md)
- [Add and manage logins](../guides/logins/add-and-manage.md)
- [Secrets and logins from Go](../guides/library/secrets-and-logins.md)
