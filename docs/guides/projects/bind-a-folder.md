# Bind a folder, step by step

This page sets up one client repo from scratch: a new home for its history, a login for its quota, and a binding so a plain `agx run` (or a bare `agx`) in that repo always uses both. Every output below was produced by these exact commands in a throwaway home (the home directory is shown as `/home/you`, with a stand-in `claude` that only prints what it was given). The words and rules are in the [Projects overview](overview.md).

## Before you start

- `agx profiles` lists your existing homes. Here they are the two discovered ones, `personal` (`~/.claude`) and `work` (`~/.claude-work`).
- A token for the login: run `claude setup-token` on the account that should pay and copy what it prints. Never paste it on a command line; pipe it or paste it at the prompt.

## Step 1 — Create a home with `agx profile add`

Preview first; `--dry-run` changes nothing:

```text
$ agx profile add client --dry-run
would create home ~/.claude-client
would add profile client (claude, ~/.claude-client)
would link CLAUDE.md -> ~/.claude/CLAUDE.md
would link skills -> ~/.claude/skills
skipped settings.json (not in the default home)
```

Then for real:

```text
$ agx profile add client
created home ~/.claude-client
agx: wrote discovered profile personal into the config
agx: wrote discovered profile work into the config
added profile client (claude, ~/.claude-client)
linked CLAUDE.md -> ~/.claude/CLAUDE.md
linked skills -> ~/.claude/skills
skipped settings.json (not in the default home)
```

The home is created private (mode 0700). Adding the first profile to the config switches discovery off for Claude, so agx writes the discovered `personal` and `work` profiles into the file too; nothing else about them changes. Options:

| Flag | Effect |
|---|---|
| `--provider codex` | a Codex home, `~/.codex-NAME`; nothing is linked |
| `--home PATH` | another folder; `~` is expanded |
| `--login NAME` | the profile's own `login:` (a name from `agx login ls`, or `home`) |
| `--setup` | after adding, run the vendor's interactive login for the new home (`claude auth login`, or `codex login`), so the home has a login of its own |
| `--bare` | link nothing |
| `--dry-run` | print the plan and change nothing |

A name already in use is refused, except a profile that exists only by discovery at the same home: naming it adopts it into the config. A folder that already exists is adopted as it is, with no links added; a file at that path is refused.

## Step 2 — Add the login that pays

```text
$ printf 'sk-ant-oat01-DUMMY-work-0000\n' | agx login add work --stdin --account work@example.com
added login work (claude, oauth-token, stored)
agx: tokens.yaml holds raw tokens: never sync, share or commit it (~/.agx/tokens.yaml)
login work: resolved ok (not probed)
see its usage with `agx login check work` (one probe: a few tokens of its quota)
```

The kind came from the prefix. Tokens that cannot run Claude are refused before anything is stored, and a `--kind` that contradicts the prefix is refused too:

```text
$ printf 'sk-ant-ort01-DUMMY\n' | agx login add bad --stdin
agx: claude: this token cannot run Claude: it is an OAuth refresh token (sk-ant-ort…); use the token `claude setup-token` prints

$ printf 'sk-ant-api03-DUMMY\n' | agx login add console --stdin
added login console (claude, api-key, stored)

$ printf 'sk-ant-api03-DUMMY\n' | agx login add console2 --stdin --kind oauth-token
agx: --kind oauth-token does not match the token, which is an api-key by its prefix
```

(The `console` add prints the same three follow-up lines as `work`; they are left out here.) Every other way to add a login, including `--cmd` for a password manager and `--env`, is in [Add and manage logins](../logins/add-and-manage.md).

## Step 3 — Bind the folder with `agx project use`

Run it in the folder to bind. Give `-p`, `--login`, or both:

```text
$ cd ~/code/client-app
$ agx project use -p client --login work
bound /home/you/code/client-app -> profile client, login work

$ cd ~/code/side
$ agx project use --login work
bound /home/you/code/side -> login work
```

The second binding keeps whatever home a run there would use and only changes whose quota it spends. Running `agx project use` again in the same folder replaces its binding and prints what it was. A login that cannot run the named profile (another provider's login, a profile billed per token) is refused here, before anything is written.

## Step 4 — Check what a run would use with `agx project show`

`show` reads the config only (no network, no token) and says which rule chose each part. From a subfolder of the bound repo:

```text
$ agx project show
folder:  /home/you/code/client-app/src
project: /home/you/code/client-app (profile client, login work)
profile: client (claude) — project binding at /home/you/code/client-app
home:    ~/.claude-client — profile client
login:   work — project binding at /home/you/code/client-app
```

It takes `-p`, `--login` and `--provider` like `agx run`, to try a combination, and `--json` for scripts ([`project.show`](../usage/json-and-scripting.md#projectshow)). A sibling folder whose name only starts the same way is not covered:

```text
$ cd ~/code/client-app-old
$ agx project show
folder:  /home/you/code/client-app-old
project: none
profile: personal (claude) — default profile
home:    ~/.claude — profile personal
login:   home — the home's own login
```

## Step 5 — Run

`--dry-run` shows the binding on its first line:

```text
$ cd ~/code/client-app/src
$ agx run --dry-run
project: /home/you/code/client-app (profile client, login work)
profile: client (claude, /home/you/.claude-client)
login:   work (project binding at /home/you/code/client-app)
dir:     /home/you/code/client-app/src
env:     CLAUDE_CODE_OAUTH_TOKEN=<token:stored>
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-client
exec:    claude
```

Flags still win, each for its own half:

```text
$ agx run -p personal --dry-run
project: /home/you/code/client-app (profile client, login work)
profile: personal (claude, /home/you/.claude)
login:   work (project binding at /home/you/code/client-app)
dir:     /home/you/code/client-app/src
env:     CLAUDE_CODE_OAUTH_TOKEN=<token:stored>
exec:    claude

$ agx run --login home --dry-run
project: /home/you/code/client-app (profile client, login work)
profile: client (claude, /home/you/.claude-client)
login:   home (--login)
dir:     /home/you/code/client-app/src
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-client
exec:    claude

$ AGX_LOGIN=home agx run --dry-run
project: /home/you/code/client-app (profile client, login work)
profile: client (claude, /home/you/.claude-client)
login:   home (AGX_LOGIN)
dir:     /home/you/code/client-app/src
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-client
exec:    claude
```

At a terminal, with no flags, a bare `agx` in the bound folder starts the agent and says why on stderr; piped, it still shows usage:

```text
$ agx
agx: project /home/you/code/client-app -> profile client, login work
[fake claude] args:  | CLAUDE_CONFIG_DIR=/home/you/.claude-client | CLAUDE_CODE_OAUTH_TOKEN is set | cwd /home/you/code/client-app

$ agx < /dev/null | head -2
personal  personal · claude  ~/.claude
  error: not logged in — run `agx run -p personal` once to log in / refresh
```

To switch the token of a conversation that is already running, exit the agent and resume it on the other login: `agx resume --last --login console`. The conversation stays in its home; only the quota changes.

## Step 6 — List, check and remove bindings

```text
$ agx project ls
PATH                       PROFILE  LOGIN  STATE
/home/you/code/client-app  client   work   ok
/home/you/code/side        —        work   ok
```

STATE names any problem (a profile or login that no longer exists, a login that cannot run the bound profile). `agx doctor` checks every binding, and `agx login ls` / `agx login rm` count bindings as users of a login:

```text
$ agx login ls
NAME     PROVIDER  KIND         SOURCE  ACCOUNT           ADDED       EXPIRES     USED-BY
work     claude    oauth-token  stored  work@example.com  2026-11-01  2027-11-01  project /home/you/code/client-app, project /home/you/code/side
console  claude    api-key      stored  —                 2026-11-01  never       —

$ agx login rm work
agx: login work is used by project /home/you/code/client-app, project /home/you/code/side — point them elsewhere (`agx login use home -p NAME`, `agx project rm PATH`) or pass --force

$ agx doctor
✓ config             ~/.agx/config.yaml (decided by env-category)
· sessions.root      ~/agx-sessions does not exist yet (created by `agx new`)
! profile personal   not logged in — run `agx run -p personal` and log in
! profile work       not logged in — run `agx run -p work` and log in
! profile client     not logged in — run `agx run -p client` and log in
✓ tokens.yaml        ~/.agx/tokens.yaml (0600)
✓ login work         oauth-token (stored), expires 2027-11-01
✓ login console      api-key (stored), no known expiry
✓ project            /home/you/code/client-app -> profile client, login work
✓ project            /home/you/code/side -> login work
· shell              shell layer not loaded — add `eval "$(agx shell-init zsh)"` to your shell rc so new/resume can cd
```

`agx login rm NAME --force` clears the login from every binding; a binding that had nothing else to bind is removed. `agx project rm` removes the binding stored for exactly the current folder (a parent's binding is left alone), or for the folder you name:

```text
$ agx project rm
removed project binding /home/you/code/client-app (profile client, login work)

$ agx project rm ~/code/side
removed project binding /home/you/code/side (login work)

$ agx project rm
agx: no project binding for /home/you/code/client-app (see `agx project ls`)
```

The last one exits 1. A binding whose folder was deleted can still be removed by the path `agx project ls` prints.

## Try it

In a throwaway home, so nothing touches your real `~/.agx` or logins. Set the two variables in two statements: in `export HOME=… AGX_CONFIG_DIR="$HOME/.agx"` the second `$HOME` is expanded before the first assignment happens, so it would point at your real config.

```sh
export HOME="$(mktemp -d)"
export AGX_CONFIG_DIR="$HOME/.agx"
mkdir -p ~/.claude/projects ~/.claude-work/projects ~/code/app
agx profile add client --bare
cd ~/code/app && agx project use -p client
agx project show
agx run --dry-run
```

```text
created home ~/.claude-client
added profile client (claude, ~/.claude-client)
agx: wrote discovered profile personal into the config
agx: wrote discovered profile work into the config
bound /home/you/code/app -> profile client
folder:  /home/you/code/app
project: /home/you/code/app (profile client)
profile: client (claude) — project binding at /home/you/code/app
home:    ~/.claude-client — profile client
login:   home — the home's own login
project: /home/you/code/app (profile client)
profile: client (claude, /home/you/.claude-client)
login:   home
dir:     /home/you/code/app
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-client
exec:    claude
```

## Pitfalls

- **Bindings do not travel with the repo.** They are in `~/.agx/config.yaml` on this machine, keyed by absolute path. A clone elsewhere, or the same repo moved to another folder, has no binding until you run `agx project use` there.
- **Aliases that name a profile keep it.** `cl: run -p personal` runs the `personal` home in a bound folder and takes only the binding's login. Use `agx run`, or a bare `agx`, to get the binding's profile.
- **A new home starts without MCP servers.** Only `CLAUDE.md`, `skills` and `settings.json` are linked; add MCP servers to the new home again. Claude Code also asks once per folder whether to trust it.

## Related

- [Projects overview](overview.md): home vs login, the precedence, auto-start rules, where credentials live, token kinds.
- [Choosing a login](../logins/choosing-a-login.md): the login precedence with the binding in it.
- [Config reference](../config-shell/config-reference.md#step-5--bind-folders-projects): what `agx project use` writes.
- [Health check](../usage/doctor.md): the `project` rows.
