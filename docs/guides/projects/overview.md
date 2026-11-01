# Projects: bind a folder to a profile and a login

If your Claude Code setup has grown by accident (a `~/.claude` from the first login, a `~/.claude-work` made by hand, a setup-token in a password manager, an API key in an environment variable) you probably type `-p` and `--login` on every run and still sometimes start a client repo on the wrong account. A **project binding** fixes that per folder: you say once which profile and which login a folder uses, and every `agx run`, `agx new` and `agx resume` in that folder (and in the folders under it) follows it without flags. This page explains the words and the rules; [Bind a folder, step by step](bind-a-folder.md) is the hands-on part.

## Before you start

- agx installed; `agx profiles` lists at least one Claude home. With no config, `~/.claude` is profile `personal` and every `~/.claude-<name>` is profile `<name>` ([Find and add accounts](../getting-started/accounts.md)).
- For a login binding, a login added with `agx login add` ([Logins overview](../logins/overview.md)). A binding by profile alone needs no login.

## Step 1 — Home and login, in plain words

A **home** is a folder on disk, such as `~/.claude-client`. It holds the conversation history, `resume` list, settings and the login the vendor CLI saved there when you ran its own login. A **profile** is agx's name for one home (`client` → `~/.claude-client`).

A **login** is a credential with a name (`work`), kept by agx, not by any home. It decides whose quota a run spends. When a run uses a login, agx puts that credential in the agent's environment and leaves the home alone, so the history stays where it was.

```text
folder ~/code/client-app ──► profile client ──► home ~/.claude-client  (history, settings, resume list)
                         └─► login work                                (whose quota the run spends)
```

The two are chosen separately. A binding can set either one or both:

| Binding | What it means | Typical use |
|---|---|---|
| `-p client` | runs here use the `client` home and its usual login | a client repo with its own history |
| `--login work` | runs here keep the usual home but spend `work` quota | your normal home, someone else's plan |
| `-p client --login work` | both | a client repo whose history lives in its own home, paid by the work plan |

## Step 2 — Know what each command uses

In a folder bound with `agx project use -p client --login work`:

| You type | Home (profile) | Login |
|---|---|---|
| `agx run` | `client` (binding) | `work` (binding) |
| `agx` at a terminal, no flags | `client` (binding) | `work` (binding); it starts the agent, see [Step 4](#step-4--know-when-a-bare-agx-starts-the-agent) |
| `agx run -p personal` | `personal` (flag) | `work` (binding) |
| `agx run --login home` | `client` (binding) | the client home's own login |
| `AGX_LOGIN=home agx run` | `client` (binding) | the client home's own login |
| `agx run --provider codex` | the default Codex profile | the Codex default (a Claude login is skipped with a note) |
| `agx resume` | the home that stores the conversation | `work` (binding) |
| `agx new topic` | `client` (binding of the folder you typed it in) | `work` (binding) |

`resume` never moves a conversation to another home: it runs in the home that holds it, and only the binding's login applies. The binding that counts is the one covering the conversation's folder, so `agx resume --all` typed anywhere still follows the client repo's binding.

## Step 3 — Learn the precedence

Profile and login are resolved independently. For each, the first rule that gives an answer wins:

| Order | Profile (the home) | Login (the quota) |
|---|---|---|
| 1 | `-p NAME` | `--login NAME` (or `home`, or `auto`) |
| 2 | the binding's profile | `AGX_LOGIN` (what `aglogin` sets for one terminal) |
| 3 | the default profile (`default: true`, or `~/.claude`) | the binding's login |
| 4 | | the profile's `login:` in config.yaml |
| 5 | | the provider's `providers.<id>.login` |
| 6 | | the home's own login |

`AGX_LOGIN` sits above the binding because `aglogin` is a choice you made for this terminal just now. `--provider codex` overrides a binding whose profile is a Claude profile; a `--provider` that matches the bound profile changes nothing.

A binding's login that does not fit the profile in use (a Claude login under a Codex profile, a login on a profile billed per token) is skipped with a one-line note when you picked that profile yourself with `-p`, and is an error when the binding's own profile cannot take it. A binding never falls back to another account silently: if it names a profile or login that no longer exists, the run stops and says how to fix it.

The closest binding wins: a binding on `~/code/client-app/tools` replaces the one on `~/code/client-app` for everything under `tools`; the two are not merged. Matching is by whole folder names, so a binding on `~/code/app` does not cover `~/code/app-old`.

## Step 4 — Know when a bare `agx` starts the agent

Plain `agx` normally shows usage for every account. In a bound folder it starts the agent instead, exactly like `agx run`, when all of these hold:

- a binding covers the current folder;
- standard input and standard output are both a terminal (not a pipe, not a file, not `/dev/null`);
- no flag was given (`agx --json`, `agx --provider codex` and friends still show usage).

It prints one line on stderr first, so you always see which account it chose:

```text
agx: project /home/you/code/client-app -> profile client, login work
```

`agx usage` always shows usage, in any folder. Scripts, status bars and `watch agx` are unaffected because they never have a terminal on both ends.

## Step 5 — Know what a new home shares

`agx profile add NAME` creates `~/.claude-NAME` (or `~/.codex-NAME` with `--provider codex`) and adds the profile to config.yaml. A new Claude home gets symlinks to these entries of `~/.claude`, when they exist there:

| Linked | Why |
|---|---|
| `CLAUDE.md` | your global instructions, kept in one place |
| `skills` | your skills |
| `settings.json` | your settings (permissions, hooks, status line) |

Nothing else is shared. Credentials, history, `projects/`, caches and the MCP server list stay per home: Claude Code keeps MCP servers in the home's own state file, so a new home starts with none and you add them again (`claude mcp add` run under that home). An entry that already exists in the new home is never replaced, `--bare` links nothing, and a home that already exists is adopted exactly as it is, with no links added. A Codex home links nothing.

## Step 6 — Know where every credential lives

| Credential | Where it is | Who writes it |
|---|---|---|
| A Claude home's own login | macOS: the login keychain (`Claude Code-credentials`, named per home); Linux and others: `<home>/.credentials.json` | `claude` when you log in (`agx profile add --setup` runs `claude auth login` for the new home) |
| A Codex home's own login | `<home>/auth.json` | `codex login` |
| A login added by paste or `--stdin` | `~/.agx/tokens.yaml`, mode 0600; config.yaml only says `token: stored` | `agx login add` |
| A login added with `--cmd` | nowhere on disk by agx; the command runs at each launch (for example `gopass show -o …`) | your password manager |
| A login added with `--env` | nowhere on disk by agx; the variable is read at each launch | your shell or CI |
| A profile's `secrets:` | your password manager (`gopass:`) or an environment variable (`env:`), resolved at launch | you |
| Bindings | `~/.agx/config.yaml`, under `projects:` | `agx project use` |

agx never refreshes, copies or prints a vendor login, and never prints a login's token.

## Step 7 — Recognise a token by its prefix

`agx login add` reads the token's prefix and sets the kind for you; `--kind` is only needed for a value it does not recognise. A `--kind` that contradicts the prefix is refused, and every time a token is resolved (when it is added, by `agx login check`, at each launch) the same check runs, so a `--cmd` or `--env` source that hands back the wrong kind of token is caught too.

| Prefix | What it is | agx |
|---|---|---|
| `sk-ant-oat…` | an OAuth access token, which is what `claude setup-token` prints | kind `oauth-token` (plan quota) |
| `sk-ant-api…` | a console API key | kind `api-key` (billed per token) |
| `sk-ant-ort…` | an OAuth refresh token | refused: it cannot run Claude, and using it elsewhere would rotate the owner's login away |
| `sk-ant-admin…` | an Admin API key | refused: it cannot run inference |

A prefix tells the kind, never the owner: two setup-tokens from different accounts look the same, which is why `agx login add --account` exists as a label for your eyes.

A setup-token is inference-only. It can run Claude, but the account, plan and usage endpoints refuse it, so the account, plan and usage of such a login come only from a probe (`agx usage --login NAME`, one tiny request that spends a few tokens of its quota). When a setup-token reaches those endpoints (for example as `CLAUDE_CODE_OAUTH_TOKEN` in a home's place), agx now says so instead of telling you to log in again:

```text
this token can only run Claude: it has no profile scope (a `claude setup-token` token), so its account, plan and usage cannot be read — add it as a login and read its usage with `agx usage --login NAME` (a small probe)
```

Any other refusal from those endpoints carries the server's own message, for example `login rejected by the server (HTTP 403: OAuth authentication is currently not allowed for this organization.)`.

## Pitfalls

- **Bindings are per machine.** They live in `~/.agx/config.yaml`, keyed by the folder's absolute path, never in a file inside the repo: which account a folder runs on is your choice, not something a clone should carry. On another machine, or after moving the repo, run `agx project use` again.
- **Symlinked folders.** The path is stored with symlinks resolved (on macOS, `/tmp` itself is a symlink), so a folder reached through a link still matches.
- **Aliases with `-p` keep their home.** An alias such as `cl: run -p personal` names a profile, so in a bound folder `cl` still runs the `personal` home and only takes the binding's login. Use plain `agx run` (or a bare `agx`) when you want the binding's profile too.
- **Changing the token mid-conversation.** A running agent keeps the credential it started with. Exit it, then continue the same conversation on another login: `agx resume --last --login work`.
- **Trust prompts.** A new home has never seen your folders, so Claude Code asks once per folder whether to trust it, as it did in your first home.
- **A binding that points at nothing.** Removing a profile from config.yaml by hand, or `agx login rm --force`, can leave a binding that names something gone. It still loads, `agx project ls` shows the problem in STATE, `agx doctor` fails on it, and a run in that folder stops with the fix. `agx login rm` without `--force` refuses while a binding uses the login.

## See also

- [Bind a folder, step by step](bind-a-folder.md): `profile add`, `project use`, `show`, `ls`, `rm`, with real output.
- [Logins overview](../logins/overview.md) and [Choosing a login](../logins/choosing-a-login.md): the login half in depth.
- [Which profile, which login](../../choosing/which-profile-or-login.md): when a binding is the right tool.
- [Config reference](../config-shell/config-reference.md#step-5--bind-folders-projects): the `projects:` key.
