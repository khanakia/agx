# Config reference

agx works with no config file at all: it discovers your Claude and Codex homes and applies built-in defaults. `~/.agx/config.yaml` is for when you want more: named profiles for extra accounts, an api-billed backend, default arguments, logins, and shell aliases. This page lists every key with its type, default and validation, explains how the file combines with discovery, and shows the exact error for each mistake. Use it when you write the file by hand or when agx refuses one.

## Before you start

- Find the file: `agx doctor` prints its path on the first line and which rule chose it (`~/.agx/config.yaml (decided by home-dotfile)`). `AGX_CONFIG_DIR` or `AGX_HOME` move it; see [Environment variables](environment.md).
- The file is optional. A missing file is not an error; an empty one is the same as a missing one.
- The file is decoded strictly: an unknown or misspelled key is an error, so typos never pass silently (`config/config.go`, `Decode`).

## Step 1 — Start from a complete file

Every key appears below. This file loads as written (it was checked with `agx profiles` and `agx doctor` against a throwaway home):

```yaml
# ~/.agx/config.yaml
sessions:
  root: ~/agx-sessions          # where `agx new` creates folders
  promote_root: ~/projects      # default destination of `agx sessions promote`

providers:
  claude:
    args: ["--permission-mode", "acceptEdits"]   # added to every Claude profile
    login: personal             # default login for every Claude profile

logins:
  - name: personal
    provider: claude
    token: stored               # the raw value is in tokens.yaml
    account: me@example.com
    added: 2026-10-01
  - name: work
    provider: claude
    kind: oauth-token
    token_cmd: gopass show -o personal/ai/claude-oauth-work
    added: 2026-10-01

profiles:
  - name: personal
    provider: claude
    home: ~/.claude
    default: true
  - name: work
    provider: claude
    home: ~/.claude-work
    args: ["--model", "opus"]
  - name: kimi
    provider: claude
    home: ~/.claude
    billing: api
    args_replace: true
    env:
      ANTHROPIC_BASE_URL: https://api.moonshot.ai/anthropic
    secrets:
      ANTHROPIC_AUTH_TOKEN: env:KIMI_API_KEY
    login: home
  - name: codex
    provider: codex
    home: ~/.codex

shell:
  aliases:
    cl: run -p personal
    clw: run -p work
    clnew: new -p personal
```

## Step 2 — Look up a key

All paths accept a leading `~`, are made absolute, and are cleaned. Top-level keys: `sessions`, `providers`, `logins`, `profiles`, `shell` (`config/config.go`, `File`).

### sessions

| Key | Type | Default | Notes |
|---|---|---|---|
| `sessions.root` | path | `~/agx-sessions` | Where `agx new` creates session folders. `agx doctor` warns if it is inside a git repo. |
| `sessions.promote_root` | path | the parent of `sessions.root` | Default destination of `agx sessions promote`. |

### providers

Keyed by provider id: `claude` or `codex`. Any other id is an error.

| Key | Type | Default | Notes |
|---|---|---|---|
| `providers.<id>.args` | list of strings | none | Prepended to every profile of that provider, including discovered ones. A profile with `args_replace: true` drops them. |
| `providers.<id>.login` | string | none | Default login for every profile of that provider (step 4 of [the login precedence](../logins/choosing-a-login.md#step-1--learn-the-precedence)). A login name of the same provider, or `home`; never `auto`. Skipped for a profile it does not fit. |

### logins

A list; each entry is one named credential. See [Add and manage logins](../logins/add-and-manage.md); `agx login add` writes these entries for you.

| Key | Type | Default | Notes |
|---|---|---|---|
| `name` | string | required | Matches `^[a-z0-9][a-z0-9-]*$`, unique, not `home` or `auto`. |
| `provider` | string | `claude` | A provider that supports logins. Only `claude` does; `codex` is refused. |
| `kind` | string | `oauth-token` | `oauth-token` (plan quota) or `api-key` (per token). |
| `token` | string | none | Only the word `stored` is accepted: the value is in `tokens.yaml`. |
| `token_cmd` | string | none | A command run with `sh -c` at each launch that prints the token. |
| `token_env` | string | none | The name of a variable that holds the token at launch. |
| `account` | string | none | A free-form label (an email), shown in listings. Not verified. |
| `added` | date `YYYY-MM-DD` | none | When the login was added; expiry is counted from it. |

Exactly one of `token`, `token_cmd`, `token_env` must be set.

### profiles

A list, in the order you want them: config order breaks auto-pick ties, and the first profile of a provider is its default unless one says `default: true`.

| Key | Type | Default | Notes |
|---|---|---|---|
| `name` | string | required | Matches `^[a-z0-9][a-z0-9-]*$`, unique, not `auto` (reserved for `-p auto`). |
| `provider` | string | required | `claude` or `codex`. |
| `home` | path | the provider's default: `~/.claude` or `~/.codex` | Several profiles may share one home. |
| `billing` | string | `plan` | `plan` or `api`. An api-billed profile has no usage windows, is never chosen by auto-pick, and cannot take a login. |
| `default` | bool | `false` | The provider's default profile. At most one per provider; with none, the first profile of the provider is the default. |
| `args` | list of strings | none | Extra arguments for the agent, after the provider's `args`. |
| `args_replace` | bool | `false` | `true` makes `args` replace the provider's `args` instead of extending them. |
| `env` | map of string to string | none | Variables set for the agent. |
| `secrets` | map of variable name to `scheme:ref` | none | Resolved at launch, never stored: `gopass:<path>` runs `gopass show -o <path>`; `env:<NAME>` reads another variable. A variable may not appear in both `env` and `secrets`. |
| `login` | string | none | The profile's default login: a login of the same provider, or `home`; never `auto`. Refused when the profile is api-billed or sets the login's credential variables itself. |

### shell

| Key | Type | Default | Notes |
|---|---|---|---|
| `shell.aliases` | map of name to agx arguments | none | One shell function per entry, generated by `agx shell-init`. The name matches `^[A-Za-z_][A-Za-z0-9_-]*$`; the value is non-empty and uses only letters, digits, spaces and `_./:=@+-`, because it is pasted into shell code. |

## Step 3 — Understand discovery

With no profile configured for a provider, agx discovers that provider's homes (`provider/claude/discover.go`, `provider/codex/codex.go`):

- `~/.claude` becomes profile `personal`; every `~/.claude-<name>` directory becomes profile `<name>`. A directory counts only if it holds `settings.json`, `projects/`, a credentials file or `.claude.json`.
- `~/.codex` with an `auth.json` becomes profile `codex` (api-billed when that login is an API key only).
- Discovered profiles still get `providers.<id>.args`.
- A discovered name that collides with a configured one gets the provider as a prefix (`codex-personal`), rather than being dropped.

**As soon as a provider has one configured profile, discovery for that provider is switched off** (`config/config.go`, `Resolve`): the config is then the complete, intentional list for it. Configuring only Claude profiles leaves Codex discovery on.

## Step 4 — Let agx edit the file for you

`agx login add`, `agx login use` and `agx login rm` edit config.yaml in place through the YAML node API: your comments and key order survive, and every edit is fully validated before it is written, so agx never writes a file it would then refuse (`config/edit.go`, `Editor`). A new file is created with mode 0600 in a 0700 folder; an existing file keeps its mode. Each edit prints what changed:

```sh
agx login use personal -p work
```

## Try it

A hand-written file with comments, then two edits (throwaway home):

```text
$ cat ~/.agx/config.yaml
# My agx setup. Comments survive `agx login add/use/rm`.
sessions:
  root: ~/agx-sessions # where `agx new` creates folders

profiles:
  # the subscription I pay for
  - name: personal
    provider: claude
    home: ~/.claude
    default: true
  # the company account
  - name: work
    provider: claude
    home: ~/.claude-work

$ printf 'sk-ant-api03-FAKE-key-5555555555\n' | agx login add ci --stdin --kind api-key --account ci@example.com
added login ci (claude, api-key, stored)
agx: tokens.yaml holds raw tokens: never sync, share or commit it (~/.agx/tokens.yaml)
login ci: resolved ok (not probed)
see its usage with `agx login check ci` (one probe: a few tokens of its quota)

$ agx login use ci -p work
~/.agx/config.yaml: profiles[work].login
- login: (unset)
+ login: ci

$ cat ~/.agx/config.yaml
# My agx setup. Comments survive `agx login add/use/rm`.
sessions:
  root: ~/agx-sessions # where `agx new` creates folders
profiles:
  # the subscription I pay for
  - name: personal
    provider: claude
    home: ~/.claude
    default: true
  # the company account
  - name: work
    provider: claude
    home: ~/.claude-work
    login: ci
logins:
  - name: ci
    provider: claude
    kind: api-key
    token: stored
    account: ci@example.com
    added: 2026-10-17
```

The complete file from Step 1, loaded:

```text
$ agx profiles
PROFILE    PROVIDER  HOME            ACCOUNT  STATE    LOGIN                BILLING  SOURCE
*personal  claude    ~/.claude                none     personal (provider)  plan     config
work       claude    ~/.claude-work           none     personal (provider)  plan     config
kimi       claude    ~/.claude                api key  home (profile)       api      config
*codex     codex     ~/.codex                 none     home                 plan     config

$ agx run -p work --dry-run
profile: work (claude, /home/you/.claude-work)
login:   personal (provider)
dir:     /home/you
env:     CLAUDE_CODE_OAUTH_TOKEN=<token:stored>
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude --permission-mode acceptEdits --model opus
```

## Pitfalls

Every message below is real agx output for the file shown (the path prefix `agx: /home/you/.agx/config.yaml: ` is trimmed). All exit 2, and every command refuses to run until the file is fixed.

| Mistake | Message |
|---|---|
| `profils: []` (typo) | `config: invalid: yaml: unmarshal errors: line 1: field profils not found in type config.File` |
| `token: sk-ant-oat01-FAKE-raw-4444` | `config: invalid: logins[0] (p): token: a raw token does not belong in config.yaml (only "stored" is accepted) — delete this login from config.yaml and move the token to tokens.yaml: `agx login add p --stdin`` |
| `token: stored` and `token_env: X` | `config: invalid: logins[0] (p): set exactly one of token: stored, token_cmd, token_env` |
| `added: 17-10-2026` | `config: invalid: logins[0] (p): added: want YYYY-MM-DD, got "17-10-2026"` |
| `kind: password` | `config: invalid: logins[0] (p): kind: "password" is not supported (want one of [oauth-token api-key])` |
| a profile named `Work` | `config: invalid: profiles[0] (Work): name must match ^[a-z0-9][a-z0-9-]*$` |
| a profile named `auto` | `config: invalid: profiles[0] (auto): "auto" is reserved for auto-pick` |
| two `default: true` for claude | `config: invalid: profiles "a" and "b" are both default for claude` |
| `billing: free` | `config: invalid: profiles[0] (a): billing must be one of [plan api]` |
| `secrets: {K: vault:x}` | `config: invalid: profiles[0] (a): secrets.K: secret: unknown scheme "vault" (want one of [gopass env])` |
| `K` in both `env` and `secrets` | `config: invalid: profiles[0] (a): K is set in both env and secrets` |
| `login: auto` on a profile | `config: invalid: profiles[0] (a): login: "auto" is accepted only as --login on run, new, resume and ws (it probes every login, spending a few tokens of each account's quota)` |
| `login: ghost` (no such login) | `config: invalid: profiles[0] (a): login: unknown login "ghost" (see `agx login ls`)` |
| `providers: {gemini: …}` | `config: invalid: providers.gemini: unknown provider (known: claude, codex)` |
| alias `bad: "run -p personal; rm -rf ~"` | `config: invalid: shell.aliases.bad: only letters, digits, spaces and _./:=@+- are allowed` |
| alias named `9x` | `config: invalid: shell.aliases.9x: not a valid shell function name` |

More traps:

- **Configuring one profile hides the others.** Add only `work` to `profiles:` and the discovered `personal` (`~/.claude`) disappears, because discovery is off for Claude now. List every home you use. `agx login use` on a config without profiles writes the discovered ones in for you, which also turns discovery off.
- **A relative path is relative to where you run agx.** `root: sessions` is made absolute against the current directory of each command (`config/config.go`, `expand`); use `~/…` or an absolute path.
- **`args_replace` drops provider args, not profile args.** In Try it, `kimi` launches plain `claude` while `work` gets both the provider's `--permission-mode acceptEdits` and its own `--model opus`.
- **The raw-token error never echoes the token.** It names the login and the fix only; the YAML decode error for `tokens.yaml` is replaced for the same reason (`login/tokens.go`, `Load`).

## Related

- [Environment variables](environment.md)
- [Shell integration](shell-integration.md)
- [Logins overview](../logins/overview.md)
- [Choosing a login](../logins/choosing-a-login.md)
