# Usage, plans and profiles

Three read commands tell you where every account stands: `agx` / `agx usage` shows how much of each plan window is used and when it resets, `agx plan` shows the plan, subscription status and start date, and `agx profiles` shows which accounts agx knows and whether each is logged in. Use them before starting long work, to choose an account, or in a status bar. Emails in the output below are placeholders.

## Before you start

- At least one logged-in Claude Code or Codex home ([Find and add accounts](../getting-started/accounts.md)).
- Network access to the vendor's usage endpoint for `agx usage` and `agx plan`. `agx profiles` reads local files only.

## Step 1 — Show usage for every account

```sh
agx
```

`agx` with no command is `agx usage`. It fetches every account at once, one read-only request each, and prints one block per account:

```text
you@example.com  Max (20x)  personal · claude  ~/.claude
  Current session    ████████████░░░░░░░░░░░░░░░░░░   41% used  resets in 2 hr 38 min
  Weekly limits
  All models         ████████████████████████░░░░░░   79% used  resets in 2 d 21 hr
  Fable              ██████████████████████████████  100% used  resets in 2 d 21 hr
  Extra usage        ███████░░░░░░░░░░░░░░░░░░░░░░░   24% used
```

The header is the account's email and plan, then the profile, the provider and the home. Each row below it is one window:

| Row | Where it comes from | Shown |
|---|---|---|
| `Current session` | Claude's 5-hour rolling window (`kind: session`) | first, no heading |
| `All models`, then one row per model (`Fable` above) | Claude's 7-day windows (`kind: weekly`, `kind: scoped`) | under "Weekly limits" |
| `Extra usage` | Claude's pay-as-you-go overage, synthesised by agx from `extra_usage.utilization` when extra usage is switched on | last, no heading, no reset time |
| `5-hour`, `Weekly`, `30-day` | Codex's primary and secondary windows, labelled by their length | no heading |

A window type agx does not know yet is still shown, under its raw group name, rather than dropped (`internal/render/render.go`, `heading`).

Bar colours follow the vendor's own severity for each window: blue for normal (or when the vendor sends none, as Codex does), yellow for warning, red for exhausted and for any severity agx does not recognise. The track is dim. Colour is on only when stdout is a terminal and `NO_COLOR` is unset; force it with `--color always` or switch it off with `--color never`. Without colour, the bar still reads correctly because filled (`█`) and empty (`░`) cells are different characters, and any usage above 0% fills at least one cell.

## Step 2 — Narrow it down

```sh
agx usage work                  # one profile
agx usage work personal         # several profiles, in this order
agx usage --provider claude     # only Claude Code accounts
agx usage --provider codex      # only Codex accounts
agx usage --timeout 5s          # give up on a slow account sooner (default 15s, per account)
agx usage --color never | less  # plain text for a pager or a log
agx usage --json                # the usage.list JSON envelope
```

The profile names are the ones `agx profiles` lists. `--timeout` bounds each account's request separately; a slow account turns into an error row and the others still print. The JSON shape is in [JSON output and scripting](json-and-scripting.md#usagelist).

Token logins (added with `agx login add`) are not shown by default, because reading a login's usage sends it a small model request that spends a few tokens. Ask for them with `agx usage --logins` (every login, after the homes) or `agx usage --login NAME` (just that one); see [Login usage and `--login auto`](../logins/usage-and-auto.md).

## Step 3 — Read the error rows

A failing account prints its error in place of its windows and never hides the other accounts:

```text
you@work.example  Max (5x)  work · claude  ~/.claude-work
  error: usage endpoint rate-limited this account; try again shortly (retry after 2m0s)
```

| Row text | Meaning | What to do |
|---|---|---|
| ``error: not logged in — run `agx run -p NAME` once to log in / refresh`` | the home has no login | run the command, log in inside the agent |
| ``error: login expired 2023-11-14 22:13:20 — run `agx run -p NAME` once to log in / refresh`` | the stored login is past its expiry | run the command; the agent refreshes its own login |
| `error: login rejected by the server (HTTP 401)` (or 403), with the same hint | the server refused the login | as above |
| `error: usage endpoint rate-limited this account; try again shortly (retry after …)` | HTTP 429; the part in brackets appears when the server sent `Retry-After` | wait that long; poll every few minutes at most |
| `billed per token — no usage windows` (dim, not an error) | an API-billed profile (`billing: api`, or a Codex home logged in with an API key only) | nothing; it has no windows |

agx never refreshes a login itself, because a refresh rotates the token and logs the agent out; it tells you to start the agent once instead. The exit status is 1 when any account failed (an API-billed row does not count as a failure), so `agx >/dev/null || echo "an account needs attention"` works in scripts. See [Exit status](json-and-scripting.md#step-4--branch-on-the-exit-status).

## Step 4 — Show plan and subscription

```sh
agx plan                  # every plan-billed account
agx plan personal         # one profile
agx subscription          # the same command
agx plan --json           # the plan.list JSON envelope
```

```text
PROFILE   PROVIDER  ACCOUNT           PLAN       STATUS  SINCE
personal  claude    you@example.com   Max (20x)  active  2025-11-25
work      claude    you@work.example  Max (20x)  active  2025-11-25
codex     codex     —                 Plus       —       —
```

For Claude Code, PLAN, STATUS and SINCE come from the account's profile endpoint. Codex reports only its plan, read from the usage response, so STATUS and SINCE show `—`, and its ACCOUNT is `—` because a Codex home keeps no email locally. The next renewal date is not shown: the login agx reads cannot see billing details, so look it up in the vendor's billing settings. A failed account shows `error (see below)` in the PLAN column and its message under the table. `--timeout` works as for `usage`; `--login` accepts only `home`.

## Step 5 — List the profiles

```sh
agx profiles              # also: agx who, agx accounts
agx profiles --json       # the profile.list JSON envelope
```

| Column | Meaning |
|---|---|
| PROFILE | the profile name; `*` marks each provider's default (used by `run` and `new` without `-p`) |
| PROVIDER | `claude` or `codex` |
| HOME | the agent's config folder |
| ACCOUNT | email and plan from the home's local files (Codex keeps no email locally, so it is empty) |
| STATE | the home's own login: `ok` (with the time left on the stored token, when known), `expired`, `none`, `api key` (an API-billed profile with no login of its own), or `error` |
| LOGIN | whose quota a run on this profile spends: `home`, or a login name with the rule that chose it in brackets, for example `personal (profile)` |
| BILLING | `plan` (usage windows, eligible for `-p auto`) or `api` (billed per token) |
| SOURCE | `config` (from `~/.agx/config.yaml`) or `discovered` |

STATE is about the home; LOGIN is about which credential a run uses. They differ only when you use [logins](../logins/overview.md). In `--json`, `login` is the STATE value and `login_name` / `login_source` are the LOGIN column.

## Try it

The same machine as above: two Claude Code homes and one Codex home, with the work account answered by HTTP 429.

```text
$ agx
you@example.com  Max (20x)  personal · claude  ~/.claude
  Current session    ████████████░░░░░░░░░░░░░░░░░░   41% used  resets in 2 hr 38 min
  Weekly limits
  All models         ████████████████████████░░░░░░   79% used  resets in 2 d 21 hr
  Fable              ██████████████████████████████  100% used  resets in 2 d 21 hr
  Extra usage        ███████░░░░░░░░░░░░░░░░░░░░░░░   24% used

you@work.example  Max (5x)  work · claude  ~/.claude-work
  error: usage endpoint rate-limited this account; try again shortly (retry after 2m0s)

you@example.com  Plus  codex · codex  ~/.codex
  5-hour             ███░░░░░░░░░░░░░░░░░░░░░░░░░░░   10% used  resets in 3 hr 12 min
  Weekly             █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets in 3 d 22 hr
$ echo $?
1
$ agx profiles
PROFILE    PROVIDER  HOME            ACCOUNT                      STATE              LOGIN  BILLING  SOURCE
*personal  claude    ~/.claude       you@example.com · Max (20x)  ok (1247 d 14 hr)  home   plan     discovered
work       claude    ~/.claude-work  you@work.example · Max (5x)  ok (1247 d 14 hr)  home   plan     discovered
*codex     codex     ~/.codex                                     ok                 home   plan     discovered
```

A machine where `~/.claude` was never logged in and `~/.claude-work` holds an expired login:

```text
$ agx usage personal work
personal  personal · claude  ~/.claude
  error: not logged in — run `agx run -p personal` once to log in / refresh

you@work.example  Max (20x)  work · claude  ~/.claude-work
  error: login expired 2023-11-14 22:13:20 — run `agx run -p work` once to log in / refresh
$ agx plan personal work
PROFILE   PROVIDER  ACCOUNT           PLAN               STATUS  SINCE
personal  claude    —                 error (see below)          
work      claude    you@work.example  error (see below)          

personal: not logged in — run `agx run -p personal` once to log in / refresh
work: login expired 2023-11-14 22:13:20 — run `agx run -p work` once to log in / refresh
```

## Pitfalls

- **One row set per home, not per profile.** Two profiles on the same home (for example an API-billed profile that points Claude Code at another backend from `~/.claude`) share one login, so `agx usage` and `agx plan` show that home once, under the profile `agx run` would choose for it (`config.Homes`, used by `runUsage` and `runPlan`). Name the profile explicitly (`agx usage kimi`) to see it.
- **The Extra usage row counts towards `-p auto`.** Auto-pick scores an account by its fullest window (`provider.Usage.MaxPercent`, `pick/pick.go`), and the synthesised Extra usage row is one of the windows (`provider/claude/parse.go`, `ParseUsage`). An account with plenty of plan room but most of its monthly overage spent scores high.
- **`--provider` is not checked.** `agx usage --provider foo` prints ``no profiles to show (run `agx doctor`)`` and exits 2 rather than naming the known providers (`internal/cli/usage.go`, `runUsage`). The valid values are `claude` and `codex`.
- **Polling gets you rate-limited.** The usage endpoints answer HTTP 429 when polled many times a minute (`provider/ratelimit.go`). Every few minutes, for example `watch -n 300 agx`, is fine.
- **`--login auto` is refused here.** `agx usage --login auto` exits 2 with ``--login auto chooses a login for a launch; to compare every login use `agx usage --logins` ``, and `--login NAME` cannot be combined with `--logins` or profile names (`usageLogin`). `agx plan` refuses every token login, because the profile endpoint does not accept them (`checkPlanLogin`, `internal/cli/plan.go`).
- **`--timeout` must be positive.** `--timeout 0s` exits 2 with `--timeout must be positive`; `--color` accepts only `auto`, `always` or `never`.

## Related

- [JSON output and scripting](json-and-scripting.md): `usage.list`, `plan.list`, `profile.list` and jq recipes.
- [Health check: agx doctor](doctor.md)
- [Troubleshooting](troubleshooting.md)
- [Auto-pick](../running/auto-pick.md): how `-p auto` uses these numbers.
- [Logins](../logins/overview.md) and [Login usage and `--login auto`](../logins/usage-and-auto.md): `--logins`, `--login NAME`.
