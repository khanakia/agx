# JSON output and scripting

Every agx command that lists something takes `--json` and prints one stable envelope instead of a table, so scripts, status bars and other tools can read it with `jq` or any JSON parser. This page shows the envelope, one real example of every kind, a set of `jq` recipes, and the exit codes to branch on. Use it whenever you call agx from a script.

## Before you start

- [jq](https://jqlang.org) 1.6 or newer for the recipes (any JSON parser works).
- The examples come from throwaway machines: emails are placeholders and home paths are shown as `/home/you`. In real output, paths are absolute and never shortened to `~`.

## Step 1 — Know the envelope

```json
{
  "schema_version": 1,
  "kind": "usage.list",
  "count": 3,
  "data": []
}
```

| Field | Meaning |
|---|---|
| `schema_version` | `1`. It versions the envelope itself (these four fields) and is bumped only when they change; the fields inside `data` are described per kind below. Ignore keys you do not know, so a new field never breaks your script. |
| `kind` | what `data` holds (table below) |
| `count` | the number of entries in `data`; absent when `data` is a single object, and also when a list is empty |
| `data` | a list of entries, or one object for `session.promote` and `version.show` (which have no `count`) |

Optional fields are left out when empty (an account without an error has no `error` key), so test for them with `jq`'s `//` or `select(.error)` rather than comparing to `""`. A `login.list` row never carries a token value, and `profile.list` lists secret variable names, never their values.

| Kind | Command |
|---|---|
| [`usage.list`](#usagelist) | `agx --json`, `agx usage --json` |
| [`plan.list`](#planlist) | `agx plan --json` |
| [`profile.list`](#profilelist) | `agx profiles --json` |
| [`conversation.list`](#conversationlist) | `agx resume --list --json` |
| [`workspace.list`](#workspacelist) | `agx ws --list --json` |
| [`session.list`](#sessionlist) | `agx sessions ls --json` |
| [`session.gc`](#sessiongc) | `agx sessions gc --json` |
| [`session.promote`](#sessionpromote) | `agx sessions promote … --json` |
| [`session.move`](#sessionmove) | `agx sessions move … --json` |
| [`login.list`](#loginlist) | `agx login ls --json` |
| [`doctor.report`](#doctorreport) | `agx doctor --json` |
| [`version.show`](#versionshow) | `agx version --json` |

## Step 2 — Read the kind you need

### usage.list

One entry per account. `windows` is absent when the account failed; `error` then holds the message the bars view prints. `extra` is the vendor's own extra-usage object, passed through unchanged; `resets_at` is `null` for a window with no scheduled reset. A token-login row (from `--logins` / `--login NAME`) has no `profile` or `home` and adds `login`, `account` and `"source": "probe"`. `data` below holds two of the three entries from that run.

```json
{
  "schema_version": 1,
  "kind": "usage.list",
  "count": 3,
  "data": [
    {
      "profile": "personal",
      "provider": "claude",
      "home": "/home/you/.claude",
      "billing": "plan",
      "email": "you@example.com",
      "plan": "Max (20x)",
      "windows": [
        {
          "kind": "session",
          "group": "session",
          "label": "Current session",
          "percent": 41,
          "severity": "normal",
          "resets_at": "2026-10-17T12:38:00Z",
          "vendor_kind": "session"
        },
        {
          "kind": "weekly",
          "group": "weekly",
          "label": "All models",
          "percent": 79,
          "severity": "warning",
          "resets_at": "2026-10-20T07:00:00Z",
          "active": true,
          "vendor_kind": "weekly_all"
        },
        {
          "kind": "scoped",
          "group": "weekly",
          "label": "Fable",
          "percent": 100,
          "severity": "exhausted",
          "resets_at": "2026-10-20T07:00:00Z",
          "vendor_kind": "weekly_scoped"
        },
        {
          "kind": "other",
          "group": "extra",
          "label": "Extra usage",
          "percent": 24.2,
          "resets_at": null,
          "vendor_kind": "extra_usage"
        }
      ],
      "extra": {
        "is_enabled": true,
        "monthly_limit": 5000,
        "used_credits": 1210,
        "utilization": 24.2,
        "currency": "USD"
      }
    },
    {
      "profile": "work",
      "provider": "claude",
      "home": "/home/you/.claude-work",
      "billing": "plan",
      "email": "you@work.example",
      "plan": "Max (5x)",
      "error": "usage endpoint rate-limited this account; try again shortly (retry after 2m0s)"
    }
  ]
}
```

`kind` is one of `session`, `weekly`, `scoped` (one model) or `other`; `group` is the section it renders under (`session`, `weekly`, `plan` for Codex, `extra`); `severity` is `normal`, `warning` or `exhausted`, or absent when the vendor sends none (Codex). `active` marks the window currently binding the account, when the vendor says so. A Codex entry looks the same with `"group": "plan"` windows labelled `5-hour` and `Weekly`.

### plan.list

```json
{
  "schema_version": 1,
  "kind": "plan.list",
  "count": 2,
  "data": [
    {
      "profile": "personal",
      "provider": "claude",
      "home": "/home/you/.claude",
      "email": "you@example.com",
      "subscription": {
        "plan": "Max (20x)",
        "status": "active",
        "billing": "stripe_subscription",
        "started_at": "2025-11-25T14:37:48.866374Z"
      }
    },
    {
      "profile": "codex",
      "provider": "codex",
      "home": "/home/you/.codex",
      "subscription": {
        "plan": "Plus"
      }
    }
  ]
}
```

A failed account has an `error` string next to an empty `subscription`.

### profile.list

`login` is the home's login state (`ok`, `expired`, `none`, `api key`, `error`); `login_name` / `login_source` say whose quota a run spends and why. `expires_at` and `credential` appear when the home holds a login. `data` below holds one of four entries.

```json
{
  "schema_version": 1,
  "kind": "profile.list",
  "count": 4,
  "data": [
    {
      "name": "work",
      "provider": "claude",
      "home": "/home/you/.claude-work",
      "billing": "plan",
      "default": false,
      "source": "discovered",
      "email": "you@work.example",
      "organization": "Work Org",
      "plan": "Max (20x)",
      "login": "expired",
      "login_name": "home",
      "login_source": "home",
      "expires_at": "2023-11-14T22:13:20Z",
      "credential": "file /home/you/.claude-work/.credentials.json"
    }
  ]
}
```

A profile with `args` or `secrets` in the config adds `args` and `secret_vars` (variable names only).

### conversation.list

`profile` is the profile `agx resume` would launch on; `profiles` lists every profile on that home.

```json
{
  "schema_version": 1,
  "kind": "conversation.list",
  "count": 1,
  "data": [
    {
      "provider": "claude",
      "home": "/home/you/.claude",
      "id": "b078a831-c2b0-4921-9b4f-23a7c8f69800",
      "title": "Fix the flaky login test",
      "dir": "/home/you/proj",
      "updated": "2026-10-17T02:50:41.185486267Z",
      "profile": "personal",
      "profiles": [
        "personal"
      ]
    }
  ]
}
```

### workspace.list

One entry per folder that has conversations; `count` inside an entry is that folder's number of conversations.

```json
{
  "schema_version": 1,
  "kind": "workspace.list",
  "count": 1,
  "data": [
    {
      "dir": "/home/you/proj",
      "updated": "2026-10-17T02:50:41.185486267Z",
      "count": 1,
      "profiles": [
        "personal"
      ]
    }
  ]
}
```

### session.list

```json
{
  "schema_version": 1,
  "kind": "session.list",
  "count": 2,
  "data": [
    {
      "name": "pglite_spike_20261015_101500",
      "path": "/home/you/agx-sessions/pglite_spike_20261015_101500",
      "created": "2026-10-15T10:15:00Z",
      "modified": "2026-10-17T02:50:41.1899489Z",
      "files": 1,
      "has_history": false
    },
    {
      "name": "20261014_090000",
      "path": "/home/you/agx-sessions/20261014_090000",
      "created": "2026-10-14T09:00:00Z",
      "modified": "2026-10-14T09:00:00Z",
      "files": 0,
      "has_history": false
    }
  ]
}
```

`created` comes from the timestamp in the folder name.

### session.gc

Every session folder with the decision about it. Without `--yes` nothing is removed; with `--yes` the entries marked `"remove": true` are deleted.

```json
{
  "schema_version": 1,
  "kind": "session.gc",
  "count": 2,
  "data": [
    {
      "folder": {
        "name": "pglite_spike_20261015_101500",
        "path": "/home/you/agx-sessions/pglite_spike_20261015_101500",
        "created": "2026-10-15T10:15:00Z",
        "modified": "2026-10-17T02:50:41.1899489Z",
        "files": 1,
        "has_history": false
      },
      "remove": false,
      "reason": "has files"
    },
    {
      "folder": {
        "name": "20261014_090000",
        "path": "/home/you/agx-sessions/20261014_090000",
        "created": "2026-10-14T09:00:00Z",
        "modified": "2026-10-14T09:00:00Z",
        "files": 0,
        "has_history": false
      },
      "remove": true,
      "reason": "empty, no history"
    }
  ]
}
```

### session.promote

A single object, so there is no `count`. `history_moved` lists the Claude history folders that moved with the folder (`null` when there was none or on a dry run).

```json
{
  "schema_version": 1,
  "kind": "session.promote",
  "data": {
    "from": "/home/you/agx-sessions/pglite_spike_20261015_101500",
    "to": "/home/you/code/pglite",
    "dry_run": true,
    "history_moved": null,
    "git_init": false
  }
}
```

### session.move

One entry per move plan (one source home to one target home). `items` are the renames, `cleanup` the emptied folders removed afterwards, `titles` the conversation titles by id. `applied` is `true` once the move ran.

```json
{
  "schema_version": 1,
  "kind": "session.move",
  "count": 1,
  "data": [
    {
      "provider": "claude",
      "from_home": "/home/you/.claude",
      "to_home": "/home/you/.claude-work",
      "dir": "/home/you/proj",
      "conversations": [
        "b078a831-c2b0-4921-9b4f-23a7c8f69800"
      ],
      "items": [
        {
          "from": "/home/you/.claude/projects/-home-you-proj/b078a831-c2b0-4921-9b4f-23a7c8f69800.jsonl",
          "to": "/home/you/.claude-work/projects/-home-you-proj/b078a831-c2b0-4921-9b4f-23a7c8f69800.jsonl"
        }
      ],
      "cleanup": [
        "/home/you/.claude/projects/-home-you-proj/memory",
        "/home/you/.claude/projects/-home-you-proj"
      ],
      "titles": {
        "b078a831-c2b0-4921-9b4f-23a7c8f69800": "Fix the flaky login test"
      },
      "dry_run": true,
      "applied": false
    }
  ]
}
```

### login.list

Never contains a token. `expires` is set for setup-token logins (counted from `added`).

```json
{
  "schema_version": 1,
  "kind": "login.list",
  "count": 1,
  "data": [
    {
      "name": "personal",
      "provider": "claude",
      "kind": "oauth-token",
      "source": "stored",
      "state": "ok",
      "account": "you@example.com",
      "added": "2026-10-16",
      "expires": "2027-10-16T00:00:00Z"
    }
  ]
}
```

### doctor.report

One entry per check: `area`, `level` (`ok`, `info`, `warn`, `fail`) and `detail`. Every area is described in [Health check: agx doctor](doctor.md). `data` below holds two of eighteen entries.

```json
{
  "schema_version": 1,
  "kind": "doctor.report",
  "count": 18,
  "data": [
    {
      "area": "config",
      "level": "ok",
      "detail": "~/.agx/config.yaml (decided by home-dotfile)"
    },
    {
      "area": "sessions.root",
      "level": "warn",
      "detail": "inside git repo /home/you — Claude Code would pool every session's memory into it"
    }
  ]
}
```

### version.show

```json
{
  "schema_version": 1,
  "kind": "version.show",
  "data": {
    "binary": "agx",
    "version": "v0.3.3",
    "source": "ldflags",
    "commit": "6314c2a406c34c2252a595a775d6820748cd8ced",
    "build_time": "2026-10-17T02:35:10Z",
    "modified": false,
    "go_version": "go1.26.4",
    "platform": "linux/amd64"
  }
}
```

## Step 3 — Use the jq recipes

```sh
# one line per account: its fullest window
agx usage --json | jq -r '.data[] | "\(.profile): \([.windows[]?.percent] | max // 0)%"'

# only the accounts that failed (expired login, rate limit, …)
agx usage --json | jq -r '.data[] | select(.error) | "\(.profile): \(.error)"'

# accounts that still have room (every window under 50%); failed accounts never match
agx usage --json | jq -r '.data[] | select(([.windows[]?.percent] | max // 100) < 50) | .profile'

# compact one-liner for a status bar or tmux
agx usage --json --provider claude | jq -r '[.data[] | "\(.profile) \([.windows[]?.percent] | max // 0 | floor)%"] | join(" · ")'

# when each account's 5-hour session window resets
agx usage --json | jq -r '.data[] | .profile as $p | .windows[]? | select(.kind=="session") | "\($p) resets \(.resets_at)"'

# plan and status per account
agx plan --json | jq -r '.data[] | "\(.profile): \(.subscription.plan // "-") \(.subscription.status // "")"'

# every doctor finding that needs attention
agx doctor --json | jq -r '.data[] | select(.level=="warn" or .level=="fail") | "\(.level) \(.area): \(.detail)"'

# name, login state and email of every profile
agx profiles --json | jq -r '.data[] | "\(.name)\t\(.login)\t\(.email // "-")"'

# the five most recent conversations anywhere
agx resume --all --list --json | jq -r '.data[:5][] | "\(.updated)  \(.title)"'
```

## Step 4 — Branch on the exit status

Every command exits with one of three codes (`internal/cli/app.go`):

| Code | Meaning |
|---|---|
| `0` | everything worked |
| `1` | a runtime failure, including "output printed but some item failed": a usage or plan row with an error, a doctor check at `fail` |
| `2` | a usage error: unknown command, flag or profile, a bad flag value, an invalid config file, nothing to show |

```sh
agx usage >/dev/null
case $? in
  0) echo "all accounts ok" ;;
  1) echo "at least one account needs attention" ;;
  2) echo "agx was called wrongly or the config is broken" ;;
esac
```

With `--json`, a code 1 still prints the complete envelope on stdout, so read it first and branch afterwards. Usage errors (code 2) print one `agx: …` line on stderr and nothing on stdout.

## Try it

The recipes against the usage and plan output shown above (personal at 100% in one window, work rate-limited, codex at 10%):

```text
$ agx usage --json | jq -r '.data[] | "\(.profile): \([.windows[]?.percent] | max // 0)%"'
personal: 100%
work: 0%
codex: 10%
$ agx usage --json | jq -r '.data[] | select(.error) | "\(.profile): \(.error)"'
work: usage endpoint rate-limited this account; try again shortly (retry after 2m0s)
$ agx usage --json | jq -r '.data[] | select(([.windows[]?.percent] | max // 100) < 50) | .profile'
codex
$ agx usage --json --provider claude | jq -r '[.data[] | "\(.profile) \([.windows[]?.percent] | max // 0 | floor)%"] | join(" · ")'
personal 100% · work 0%
$ agx usage --json | jq -r '.data[] | .profile as $p | .windows[]? | select(.kind=="session") | "\($p) resets \(.resets_at)"'
personal resets 2026-10-17T12:38:00Z
codex resets 2026-10-17T13:12:00Z
$ agx plan --json | jq -r '.data[] | "\(.profile): \(.subscription.plan // "-") \(.subscription.status // "")"'
personal: Max (20x) active
work: Max (20x) active
codex: Plus 
```

## Pitfalls

- **A failed account reads as 0% in "fullest window" recipes.** It has no `windows`, so `max // 0` turns it into `0%`, as `work: 0%` shows above. Pair those recipes with the `select(.error)` one, or use `// 100` as the "still has room" recipe does, so a failure never looks like an idle account.
- **`count` is missing on single-object kinds and on empty lists.** The envelope's `count` is `omitempty` (voltkit `output.Envelope`), so `session.promote` and `version.show` never have it, and neither does an empty listing: on a machine with no accounts, `agx profiles --json` prints `{"schema_version": 1, "kind": "profile.list", "data": []}`. Use `.data | length`, not `.count`, when the number matters.
- **Paths are absolute in JSON.** The text views shorten your home to `~`; JSON keeps the full path, except where a doctor `detail` string contains a shortened one.
- **Do not match on error text.** The `error` strings carry hints such as ``— run `agx run -p work` once to log in / refresh`` and may change wording; test for the presence of `error`, and use the exit status for control flow.
- **The Extra usage window is a real entry in `windows`.** It has `"group": "extra"`, `"kind": "other"` and no `resets_at`; filter it out (`select(.group != "extra")`) if you want plan windows only (`provider/claude/parse.go`).

## Related

- [Usage, plans and profiles](usage-and-plans.md)
- [Health check: agx doctor](doctor.md)
- [Resume](../running/resume.md) and [Workspace picker](../running/workspace-picker.md): `conversation.list`, `workspace.list`.
- [Session folders](../running/new-and-sessions.md): `session.list`, `session.gc`, `session.promote`.
- [Move history](../running/move-history.md): `session.move`.
- [Logins](../logins/overview.md): `login.list`.
