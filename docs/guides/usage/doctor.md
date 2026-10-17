# Health check: agx doctor

`agx doctor` checks everything agx depends on, on this machine only: which config file it loaded and why, the sessions folder, every profile's agent binary, home and login, the private token file, every named login and its expiry, and whether the shell layer is loaded. It never touches the network and never resolves a secret or token value, so it is safe to run any time. Run it after installing, after editing `~/.agx/config.yaml`, and whenever another command fails in a way you do not understand.

## Before you start

- agx installed ([Install agx](../getting-started/README.md)). Nothing else: doctor works with no config and no accounts.

## Step 1 — Run it

```sh
agx doctor
```

Each line is one finding: a mark, an area, and a detail. The marks are `✓` ok, `·` info, `!` warn and `✗` fail. Doctor exits 1 when any finding is `✗`, and 0 otherwise, warnings included.

## Step 2 — Fix what it reports

The checks run in this order. Every row below is a message doctor can print (`internal/cli/doctor.go`); `NAME`, `PATH` and `VAR` stand for the real values.

### config

| Level | Detail | Meaning and fix |
|---|---|---|
| ✓ | `~/.agx/config.yaml (decided by home-dotfile)` | the file that was loaded, and the rule that chose its folder: `env-category` (`AGX_CONFIG_DIR`), `env-home` (`AGX_HOME`, file at `$AGX_HOME/config/config.yaml`) or `home-dotfile` (`~/.agx`) |
| · | `no file at ~/.agx/config.yaml — using discovered profiles and defaults` | normal without a config; agx uses the homes it finds on disk |
| ✗ | `PATH: config: invalid: …` | the config file does not load (unknown key, bad value, raw token in `token:`). Doctor stops here and prints nothing else; fix the line the message names |

### sessions.root

| Level | Detail | Meaning and fix |
|---|---|---|
| ✓ | `~/agx-sessions` | the folder exists |
| · | ``~/agx-sessions does not exist yet (created by `agx new`)`` | normal before the first `agx new` |
| ! | `inside git repo PATH — Claude Code would pool every session's memory into it` | the sessions folder sits inside a git work tree, so Claude Code treats every session as part of that repository. Move `sessions.root` outside any repo |
| ✗ | `PATH is not a directory` | `sessions.root` points at a file; point it at a folder |

### profile NAME

One or more lines per profile, configured or discovered. The secret checks come first and are always reported; after them, the first failure among binary, home and login ends that profile's checks.

| Level | Detail | Meaning and fix |
|---|---|---|
| ✗ | `VAR needs gopass, which is not on PATH` | a `secrets:` entry uses `gopass:…`; install gopass or put it on `PATH` |
| ! | ``ANTHROPIC_API_KEY is set in this shell, so Claude Code bills the API instead of this plan — unset it, or give the profile a login (`agx login use`)`` (or `ANTHROPIC_AUTH_TOKEN`) | a plan-billed Claude profile that runs on its home login would inherit the variable, and Claude Code then bills the API key instead of the plan; agx removes these variables only for token logins and does not touch them otherwise |
| ✗ | `claude is not on PATH` (or `codex`) | the agent binary is missing; install it or fix `PATH` |
| ! | `home PATH does not exist` | the profile's `home:` folder is missing; log in once with that home, or fix the path |
| ✓ | `you@example.com Max (20x)` | logged in, with the email and plan from the home's files; Codex shows `logged in` because it keeps no email locally |
| ✓ | `api-billed (secrets: [VAR])` | a `billing: api` profile without a login of its own; the variable names it fills at launch |
| ! | ``login expired — run `agx run -p NAME` once to refresh`` | the home's stored login is past its expiry; start the agent once and it refreshes its own login (agx never does) |
| ! | ``not logged in — run `agx run -p NAME` and log in`` | the home has no login |
| ✗ | any other message | the home's login files could not be read; the message says which |

### tokens.yaml

Checked only when `~/.agx/tokens.yaml` (beside `config.yaml`) exists, that is after `agx login add` has stored a token.

| Level | Detail | Meaning and fix |
|---|---|---|
| ✓ | `~/.agx/tokens.yaml (0600)` | private to you |
| ✗ | `login: token file is readable by others: PATH is mode 0644 — fix with: chmod 600 PATH` | group or other can read it; run the `chmod` shown. If the file was readable on a shared machine or synced, revoke and re-add the tokens. Every stored login then fails too (below) |
| ! | `FOLDER is mode 0755; keep it private: chmod 700 FOLDER` | the folder holding it is open to others |
| ! | `inside git work tree PATH — raw tokens must never be committed` | the config folder is inside a repository; make sure tokens.yaml is ignored, or move the config folder |
| ✗ | `login: PATH is not a name: token map` | the file is not a `name: "token"` mapping; re-add the logins with `agx login add NAME --stdin --replace` |

### login NAME

One or two lines per login in `config.yaml`: the token source first, then the expiry. A `token_env` login whose variable is unset gets only the warning row.

| Level | Detail | Meaning and fix |
|---|---|---|
| ✗ | `token unreadable: ~/.agx/tokens.yaml was refused (see above)` | a stored login whose file failed a tokens.yaml check; fix that first |
| ✗ | ``no token in ~/.agx/tokens.yaml — run `agx login add NAME --stdin --replace` `` | the login is in config.yaml but its token is not in tokens.yaml; add it again |
| ! | `VAR is not set in this shell` | a `token_env` login whose variable is missing in this shell; export it where you run agx |
| ✓ | `oauth-token (stored), expires 2027-10-16` | kind, source and expiry (setup-tokens are counted as valid for 365 days from `added`) |
| ✓ | `oauth-token (env), expires approximately 2027-10-01` | for `token_cmd` / `token_env` logins agx knows only when the login was added, not when the token was made, so the date is approximate |
| ✓ | `api-key (stored), no known expiry` | API keys live until revoked |
| ! | `expires 2026-11-01 (in 14 d 21 hr); renew: agx login add NAME --setup --replace` | less than 30 days left |
| ✗ | `expired 2026-10-01; renew: agx login add NAME --setup --replace` | a stored setup-token past its date; runs on it are refused until renewed |
| ! | `past its approximate expiry 2026-09-01; runs go ahead — if the token was not rotated, renew: agx login add NAME --replace` | a `token_cmd` / `token_env` login past its approximate date; it is a reminder, runs still work if your store holds a newer token |

### shell

| Level | Detail | Meaning and fix |
|---|---|---|
| ✓ | `shell layer active (zsh)` | `AGX_SHELL_INIT` is set by the shell layer |
| · | ``shell layer not loaded — add `eval "$(agx shell-init zsh)"` to your shell rc so new/resume can cd`` | add the line and open a new terminal; see [Shell integration](../config-shell/shell-integration.md) |

## Step 3 — Use it from a script

```sh
agx doctor --json | jq -r '.data[] | select(.level=="warn" or .level=="fail") | "\(.level) \(.area): \(.detail)"'
agx doctor >/dev/null || echo "agx doctor found a failure"
```

`--json` prints a [`doctor.report`](json-and-scripting.md#doctorreport) envelope with one `{area, level, detail}` entry per line above.

## Try it

A machine set up to trip most checks: the home folder is a git repository, `~/.claude-work` is missing, the `kimi` profile needs gopass (not installed), the Codex binary is missing, `~/.agx` is mode 0755, and the logins cover every expiry case.

```text
$ agx doctor
✓ config             ~/.agx/config.yaml (decided by home-dotfile)
! sessions.root      inside git repo /home/you — Claude Code would pool every session's memory into it
✓ profile personal   you@example.com Max (20x)
! profile work       home /home/you/.claude-work does not exist
✗ profile kimi       ANTHROPIC_AUTH_TOKEN needs gopass, which is not on PATH
✓ profile kimi       you@example.com Max (20x)
✗ profile codex      codex is not on PATH
✓ tokens.yaml        ~/.agx/tokens.yaml (0600)
! tokens.yaml        /home/you/.agx is mode 0755; keep it private: chmod 700 /home/you/.agx
! tokens.yaml        inside git work tree /home/you — raw tokens must never be committed
✓ login personal     oauth-token (stored), expires 2027-10-16
✗ login old          expired 2026-10-01; renew: agx login add old --setup --replace
! login soon         expires 2026-11-01 (in 14 d 21 hr); renew: agx login add soon --setup --replace
✗ login lost         no token in ~/.agx/tokens.yaml — run `agx login add lost --stdin --replace`
! login ci           AGX_DOCS_CI_TOKEN is not set in this shell
! login vault        past its approximate expiry 2026-09-01; runs go ahead — if the token was not rotated, renew: agx login add vault --replace
· shell              shell layer not loaded — add `eval "$(agx shell-init zsh)"` to your shell rc so new/resume can cd
$ echo $?
1
```

The same machine after `chmod 644 ~/.agx/tokens.yaml` (only the token lines shown): every stored login now fails, while the env and cmd logins are unaffected.

```text
✗ tokens.yaml        login: token file is readable by others: /home/you/.agx/tokens.yaml is mode 0644 — fix with: chmod 600 /home/you/.agx/tokens.yaml
! tokens.yaml        /home/you/.agx is mode 0755; keep it private: chmod 700 /home/you/.agx
! tokens.yaml        inside git work tree /home/you — raw tokens must never be committed
✗ login personal     token unreadable: ~/.agx/tokens.yaml was refused (see above)
✗ login old          token unreadable: ~/.agx/tokens.yaml was refused (see above)
✗ login soon         token unreadable: ~/.agx/tokens.yaml was refused (see above)
✗ login lost         token unreadable: ~/.agx/tokens.yaml was refused (see above)
! login ci           AGX_DOCS_CI_TOKEN is not set in this shell
! login vault        past its approximate expiry 2026-09-01; runs go ahead — if the token was not rotated, renew: agx login add vault --replace
```

A broken config hides everything else:

```text
$ agx doctor
✗ config             /home/you/.agx/config.yaml: config: invalid: profiles[2] (kimi): secrets.EXTRA_KEY: secret: unknown scheme "vault" (want one of [gopass env])
```

## Pitfalls

- **A config error is the only line you get.** When `config.yaml` does not load, doctor reports that one failure and returns (`doctor`, the early `return out` after `a.config()`). Fix it and run doctor again to see the rest.
- **A bad secret reference is a config error, not a profile finding.** `secrets:` values are parsed when the config loads (`config.Resolve` via `secret.Parse`), so an unknown scheme such as `vault:` shows up as the `config` failure above, never as a `profile NAME` line.
- **One profile can print both ✗ and ✓.** The secret checks do not stop the profile's other checks, so `profile kimi` above fails on gopass and still reports its login (by design: "Config-only checks first … must be reported even when those are broken", `doctor.go`).
- **A missing binary hides the login state.** After `claude is not on PATH` doctor skips that profile's home and login checks (`continue`), so fix `PATH` and run it again.
- **Doctor checks the shell it runs in.** `VAR is not set in this shell` and `shell layer not loaded` describe the current terminal; a login variable exported only in another shell or in a service's environment still shows as missing here.
- **Some paths are printed in full.** The home, git-repo and folder-mode details show absolute paths, while most others shorten your home to `~`.

## Related

- [Troubleshooting](troubleshooting.md)
- [Find and add accounts](../getting-started/accounts.md)
- [Config reference](../config-shell/config-reference.md): `sessions.root`, `secrets`.
- [Environment variables](../config-shell/environment.md): `AGX_CONFIG_DIR`, `AGX_HOME`.
- [Add and manage logins](../logins/add-and-manage.md): tokens.yaml rules and renewing setup-tokens.
- [Shell integration](../config-shell/shell-integration.md)
