# Add and manage logins

`agx login` adds, lists, checks, rotates and removes named logins, and keeps the raw tokens in a private file apart from your config. Use this page when you set up a login for the first time, rotate a token, move a login from a password manager into agx, or clean one up. For what a login is, read [the overview](overview.md) first.

## Before you start

- A Claude profile that agx sees (`agx profiles`).
- A token from `claude setup-token` (plan quota) or a console API key (per-token billing). Only fake values appear on this page; they look like `sk-ant-oat01-FAKE-…`.
- Know where agx keeps its files: `~/.agx/config.yaml` and, beside it, `~/.agx/tokens.yaml`. `AGX_CONFIG_DIR` or `AGX_HOME` moves both; see [Environment variables](../config-shell/environment.md).

## Step 1 — Choose where the token comes from

Every login has exactly one source. There is deliberately no `--token VALUE` flag: argv shows in `ps` and in shell history.

| How you add it | Source | Where the value lives | Read when |
|---|---|---|---|
| `agx login add NAME` (hidden prompt) | `stored` | `tokens.yaml` | at launch |
| `… --stdin` | `stored` | `tokens.yaml` | at launch |
| `… --setup` (runs `claude setup-token`, then the hidden prompt) | `stored` | `tokens.yaml` | at launch |
| `… --cmd "COMMAND"` | `cmd` | nowhere in agx; `sh -c COMMAND` prints it | at each launch |
| `… --env VAR` | `env` | nowhere in agx; another env variable | at each launch |

## Step 2 — Add a stored login

Paste at a hidden prompt:

```sh
agx login add personal
```

Or pipe it in, with an account label for your own eyes:

```sh
pbpaste | agx login add personal --stdin --account you@example.com
```

Or let agx run `claude setup-token` first and then ask for the paste:

```sh
agx login add personal --setup
```

agx trims surrounding whitespace and refuses an empty value or one with more than one non-empty line (`login/login.go`, `Clean`), stores it, and immediately resolves it once without spending anything. `--setup` cannot be combined with `--stdin`.

## Step 3 — Add a login that stores nothing

Read the token from a password manager at each launch. The command runs with `sh -c`, no stdin, your home as the working directory, and a 10-second limit:

```sh
agx login add work --cmd "gopass show -o personal/ai/claude-oauth-work"
```

Or read it from another variable your environment already sets:

```sh
agx login add ci --env CLAUDE_WORK_TOKEN
```

Neither writes anything to `tokens.yaml`. A command that fails shows its stderr with any line of its stdout masked as `<redacted>`, so a `set -x` trace cannot leak the token into agx's error (`login/login.go`, `redact`).

## Step 4 — Pick a kind

The default kind is `oauth-token`: plan quota, delivered as `CLAUDE_CODE_OAUTH_TOKEN`. An API key is billed per token and delivered as `ANTHROPIC_API_KEY`:

```sh
pbpaste | agx login add billing --stdin --kind api-key --account billing@example.com
```

## Step 5 — List and check

```sh
agx login ls
agx login ls --json | jq -r '.data[] | "\(.name)\t\(.source)\t\(.expires // "never")"'
agx login check --no-probe
```

`login ls` never prints a value. `SOURCE` shows `stored (missing)` when config says stored but `tokens.yaml` has no entry, and `stored (unreadable)` when agx refused to read the file. `EXPIRES` is counted from `added`, with `~` for `cmd` and `env` logins (Step 8). `USED-BY` lists the profiles and `providers.<id>` defaults pointing at the login.

`login check NAME` resolves the token and then probes its usage, which spends a few tokens of that account's quota; `--no-probe` only resolves. Without a name it checks every login and exits 1 if any failed. The probe is covered in [Login usage and `--login auto`](usage-and-auto.md).

## Step 6 — Rotate a token

```sh
pbpaste | agx login add personal --stdin --replace
```

`--replace` swaps the token and resets `added` to today, so expiry counts from the new token. The kind and account are kept unless you pass them again, so rotating an api-key login does not silently turn it into an oauth-token one (`internal/cli/login.go`, `runLoginAdd`).

## Step 7 — Keep tokens.yaml private

agx writes `tokens.yaml` atomically (a 0600 temp file renamed over the old one), inside a folder it creates with mode 0700. The file looks like this:

```yaml
# agx raw login tokens — written by `agx login add`; mode 0600.
# Never sync, back up to a shared place, or commit this file.
personal: "sk-ant-oat01-FAKE-personal-0000000000"
```

The rules, all in `login/tokens.go` and `internal/cli/doctor.go`:

- agx refuses to read a `tokens.yaml` that group or others can read or write, the way ssh treats a private key, and the error carries the `chmod 600` that fixes it.
- `agx doctor` warns when the folder holding it is not private (`chmod 700`), and when it sits inside a git work tree.
- `config.yaml` holds only `token: stored`, so it stays safe to sync or share. A raw token pasted into `config.yaml` makes every command refuse the file, without echoing the token (`config/logins.go`, `resolveLogin`).

## Step 8 — Renew before expiry

A setup-token is believed to last one year (not verified by the vendor) and agx never refreshes it (`provider/claude/login.go`, `SetupTokenLifetime`). API keys have no known expiry.

- `agx login ls` shows `EXPIRES` = `added` + one year.
- `agx doctor` warns 30 days ahead.
- Past that date a **stored** login is refused at launch and by `login check`, and doctor fails, with the command that renews it: `agx login add NAME --setup --replace`.
- A **cmd** or **env** login only warns and runs, and doctor warns instead of failing: agx knows when you added the login, not when the token behind it was minted, and you may have rotated it in your store since.

## Step 9 — Remove a login

```sh
agx login rm personal
```

Refused while a profile or provider default uses it; `--force` removes it anyway and clears those `login:` keys. For a stored login the entry in `tokens.yaml` goes too. Then revoke the token where it was issued.

## Try it

A throwaway home (`/home/you`), three logins, then an expired one:

```text
$ printf 'sk-ant-oat01-FAKE-personal-0000000000\n' | agx login add personal --stdin --account me@example.com
added login personal (claude, oauth-token, stored)
agx: tokens.yaml holds raw tokens: never sync, share or commit it (~/.agx/tokens.yaml)
login personal: resolved ok (not probed)
see its usage with `agx login check personal` (one probe: a few tokens of its quota)

$ agx login add work-api --kind api-key --env WORK_API_KEY
added login work-api (claude, api-key, env)
login work-api: failed: login work-api: env WORK_API_KEY is not set

$ agx login add viacmd --cmd 'printf sk-ant-oat01-FAKE-cmd-1111111111'
added login viacmd (claude, oauth-token, cmd)
login viacmd: resolved ok (not probed)
see its usage with `agx login check viacmd` (one probe: a few tokens of its quota)

$ agx login ls
NAME      PROVIDER  KIND         SOURCE  ACCOUNT         ADDED       EXPIRES      USED-BY
personal  claude    oauth-token  stored  me@example.com  2026-10-17  2027-10-17   —
work-api  claude    api-key      env     —               2026-10-17  never        —
viacmd    claude    oauth-token  cmd     —               2026-10-17  ~2027-10-17  —

$ cat ~/.agx/config.yaml
logins:
  - name: personal
    provider: claude
    token: stored
    account: me@example.com
    added: 2026-10-17
  - name: work-api
    provider: claude
    kind: api-key
    token_env: WORK_API_KEY
    added: 2026-10-17
  - name: viacmd
    provider: claude
    token_cmd: printf sk-ant-oat01-FAKE-cmd-1111111111
    added: 2026-10-17
```

The `login add work-api` line exits 1 because `WORK_API_KEY` was not set in that shell; the login is still added. With logins added on 2025-09-01 (one stored, one cmd):

```text
$ agx login check old --no-probe
login old: failed: login old expired on 2026-09-01; renew: agx login add old --setup --replace

$ agx login check oldcmd --no-probe
agx: login oldcmd is past its approximate expiry (2026-09-01, counted from when it was added); running anyway — if the token was not rotated, renew: agx login add oldcmd --replace
login oldcmd: resolved ok (not probed)
```

Rotation keeps the kind and the account:

```text
$ printf 'sk-ant-api03-FAKE-key-5555555555\n' | agx login add ci --stdin --kind api-key --account ci@example.com
added login ci (claude, api-key, stored)
agx: tokens.yaml holds raw tokens: never sync, share or commit it (~/.agx/tokens.yaml)
login ci: resolved ok (not probed)
see its usage with `agx login check ci` (one probe: a few tokens of its quota)

$ printf 'sk-ant-api03-FAKE-key-6666666666\n' | agx login add ci --stdin --replace
replaced login ci (claude, api-key, stored)
agx: tokens.yaml holds raw tokens: never sync, share or commit it (~/.agx/tokens.yaml)
login ci: resolved ok (not probed)
see its usage with `agx login check ci` (one probe: a few tokens of its quota)
```

## Pitfalls

- **A readable tokens.yaml blocks every stored login.** After `chmod 644 ~/.agx/tokens.yaml`, `agx login ls` prints `agx: login: token file is readable by others: /home/you/.agx/tokens.yaml is mode 0644 — fix with: chmod 600 /home/you/.agx/tokens.yaml` and marks stored logins `stored (unreadable)`; launches on them fail (`login/tokens.go`, `CheckPerm`).
- **The added date is the expiry clock.** Editing `added:` by hand moves the expiry agx shows and enforces; it must be `YYYY-MM-DD` or the config is refused (`added: want YYYY-MM-DD, got "17-10-2026"`).
- **The login is added even when the first resolve fails.** As in Try it, `login add --env VAR` with `VAR` unset still writes the login and then reports `failed` with exit 1. Set the variable where the agent runs, not only where you added it; `agx doctor` warns `VAR is not set in this shell`.
- **One source per login.** `agx login add bad --cmd x --env Y` is refused with `give one token source: --cmd, --env, or a paste (--stdin / --setup / the prompt)`. In `config.yaml`, two of `token`, `token_cmd`, `token_env` give `set exactly one of token: stored, token_cmd, token_env` (`config/logins.go`).
- **Reserved and invalid names.** `home` and `auto` are reserved (`login name "home" is reserved`), and names must match `^[a-z0-9][a-z0-9-]*$` (`config/logins.go`, `ValidLoginName`).
- **Adding an existing name needs `--replace`.** Otherwise: `login "personal" already exists (use --replace to rotate its token)`.
- **Multi-line input is refused.** `printf 'a\nb\n' | agx login add two --stdin` fails with `login: token has more than one line`; the same check (`login.ErrMultiline`) applies to pasted, stored, command and env tokens.
- **A token command has 10 seconds.** A store that waits for a passphrase nobody types fails the launch with `timed out after 10s` (`login/login.go`, `cmdTimeout`).
- **`login use` and `login rm` edit config.yaml in place.** Comments and key order are kept, but blank lines between sections are not (`config/edit.go`).

## Related

- [Logins overview](overview.md)
- [Choosing a login](choosing-a-login.md)
- [Login usage and `--login auto`](usage-and-auto.md)
- [Config reference: `logins`](../config-shell/config-reference.md#logins)
