# Troubleshooting

This page lists the messages agx prints when something is wrong, what each one means, and the fix. Every message here was produced by running agx; `NAME`, `PATH` and similar stand for your own values. Start with `agx doctor`, because most problems show up there first; come here when a command prints an error you do not recognise or shows fewer accounts than you expect.

## Before you start

- agx installed ([Install agx](../getting-started/README.md)).
- Know the exit status of the failing command (`echo $?` right after it): `2` means agx was called wrongly or the config file is broken, `1` means something failed at run time (often only one account), `0` means it worked.

## Step 1 — Run the health check

```sh
agx doctor
```

Fix every `✗` line first, then the `!` lines. Each message is explained in [Health check: agx doctor](doctor.md). If doctor prints only a `config` line, the config file does not load; see Step 3.

## Step 2 — An account is missing or looks logged out

| Symptom | Cause | Fix |
|---|---|---|
| `agx profiles` prints only the header; `agx` says ``no profiles to show (run `agx doctor`)`` (exit 2) | no home was found and no profile is configured | log in with Claude Code or Codex once; see [Find and add accounts](../getting-started/accounts.md) |
| a `~/.claude-NAME` folder is not listed | it holds none of `settings.json`, `projects`, `.credentials.json`, `.claude.json`, so it is not treated as an account | start Claude Code once with `CLAUDE_CONFIG_DIR=~/.claude-NAME claude` and log in |
| a second Codex home (`~/.codex-work`) is not listed | only `~/.codex` is discovered | add it to `profiles:` in `~/.agx/config.yaml`, together with `~/.codex` |
| discovered homes vanished after you added one profile | configuring any profile of a provider switches off discovery for that provider | list every home of that provider in the config |
| a profile is called `claude-personal` | a configured profile of another provider already uses `personal` | rename one of them in the config |
| `STATE none` / `error: not logged in` for `~/.claude` although Claude Code works | `CLAUDE_CONFIG_DIR` is set to `~/.claude` in your shell, so Claude Code reads a different Keychain entry | unset the variable for the default home; agx itself removes it when launching that home |
| `STATE expired` / ``error: login expired DATE — run `agx run -p NAME` once to log in / refresh`` | the home's stored login is past its expiry | run the command: the agent refreshes its own login; agx never refreshes it, because that would log the agent out |
| ``error: login rejected by the server (HTTP 401) — run `agx run -p NAME` once to log in / refresh`` (or 403) | the server refused the stored login | as above |

## Step 3 — The config file does not load

Every command refuses an invalid config with exit 2 and names the file and the line.

| Message | Fix |
|---|---|
| `…: line 5: field defualt not found in type config.ProfileSpec` | an unknown key, usually a typo; unknown keys are always an error |
| `…: profiles[2] (kimi): secrets.EXTRA_KEY: secret: unknown scheme "vault" (want one of [gopass env])` | a secret reference must be `gopass:<path>` or `env:<NAME>` |
| ``…: logins[0] (x): token: a raw token does not belong in config.yaml (only "stored" is accepted) — delete this login from config.yaml and move the token to tokens.yaml: `agx login add x --stdin` `` | a token was pasted into `config.yaml`. Delete that login's entry by hand, re-add it with the command shown, and revoke the token if the file was ever synced or committed |

The message never repeats the token value.

## Step 4 — A command refuses its arguments

| Message | Fix |
|---|---|
| `unknown command "NAME" for "agx"` / `unknown flag: --NAME` | check `agx --help` or `agx COMMAND --help`; every command lists its flags and examples |
| ``unknown profile "NAME" (see `agx profiles`)`` | use a name from `agx profiles` |
| `--color must be auto, always or never (got "pink")` | one of those three |
| `--timeout must be positive` | for example `--timeout 5s` |
| ``no plan-billed profiles (run `agx doctor`)`` | `agx plan` covers only `billing: plan` profiles; API-billed ones have no subscription to show |
| ``plan cannot show a token login: the profile endpoint refuses setup-tokens; use --login home or no --login (see a login's usage with `agx usage --login NAME`)`` | drop `--login`, or use `agx usage --login NAME` |
| `"auto" is accepted only as --login on run, new, resume and ws (it probes every login, spending a few tokens of each account's quota)` | `--login auto` belongs on a launch command |
| ``--login auto chooses a login for a launch; to compare every login use `agx usage --logins` `` | use `agx usage --logins` |
| ``unknown login "NAME" (see `agx login ls`)`` | use a name from `agx login ls` |

## Step 5 — Usage shows an error for one account

The other accounts still print and the command exits 1.

| Row | Fix |
|---|---|
| `error: usage endpoint rate-limited this account; try again shortly (retry after 2m0s)` | the vendor answered HTTP 429, usually because something polls agx too often. Wait the time shown and poll every few minutes at most (`watch -n 300 agx`) |
| `billed per token — no usage windows` (dim) | not an error: an API-billed profile, or a Codex home logged in with an API key only |
| a request or timeout error | the account did not answer within `--timeout` (15s by default) or the network failed; try `agx usage NAME --timeout 30s` |

## Try it

A first run where `~/.claude` was never logged in and `~/.claude-work` holds an expired login; doctor points at both, and usage gives the same fix:

```text
$ agx doctor
· config             no file at ~/.agx/config.yaml — using discovered profiles and defaults
· sessions.root      ~/agx-sessions does not exist yet (created by `agx new`)
! profile personal   not logged in — run `agx run -p personal` and log in
✓ profile side       you@side.example Pro
! profile work       login expired — run `agx run -p work` once to refresh
✓ profile codex      logged in
· shell              shell layer not loaded — add `eval "$(agx shell-init zsh)"` to your shell rc so new/resume can cd
$ agx usage personal work
personal  personal · claude  ~/.claude
  error: not logged in — run `agx run -p personal` once to log in / refresh

you@work.example  Max (20x)  work · claude  ~/.claude-work
  error: login expired 2023-11-14 22:13:20 — run `agx run -p work` once to log in / refresh
$ echo $?
1
```

A raw token pasted into the config stops every command:

```text
$ agx profiles
agx: /home/you/.agx/config.yaml: config: invalid: logins[0] (x): token: a raw token does not belong in config.yaml (only "stored" is accepted) — delete this login from config.yaml and move the token to tokens.yaml: `agx login add x --stdin`
$ echo $?
2
```

## Pitfalls

- **Warnings do not fail doctor.** `agx doctor` exits 0 with `!` lines present; only `✗` makes it exit 1 (`newDoctorCmd`, `internal/cli/doctor.go`). Read the output, not just the exit status.
- **Exit 1 does not mean nothing printed.** `agx usage` and `agx plan` print every account and then exit 1 if any one failed (`errPartial`, `internal/cli/app.go`); an API-billed row does not count as a failure.
- **`--provider` is not validated.** A misspelt provider prints ``no profiles to show (run `agx doctor`)`` instead of naming `claude` and `codex` (`runUsage`, `internal/cli/usage.go`).
- **Colour disappears in pipes.** `--color auto` (the default) colours only a terminal and honours `NO_COLOR` (`wantColor`); use `--color always` to keep it through `less -R`.
- **The renewal date is never shown.** `agx plan` shows plan, status and start date only; the login agx reads cannot see billing details (`newPlanCmd` help text).
- **Rate limits come from polling.** A status bar that runs `agx` every few seconds gets accounts answered with HTTP 429 (`provider.RateLimitError`); slow it down rather than retrying in a loop.

## Related

- [Health check: agx doctor](doctor.md)
- [Usage, plans and profiles](usage-and-plans.md)
- [JSON output and scripting](json-and-scripting.md): exit status and error fields.
- [Find and add accounts](../getting-started/accounts.md)
- [Config reference](../config-shell/config-reference.md)
- [Logins](../logins/overview.md) and [Add and manage logins](../logins/add-and-manage.md)
