# Login usage and `--login auto`

A home's usage comes from a free endpoint, but a setup-token cannot read that endpoint (it answers 403). So agx reads a login's usage with a **probe**: one minimal request whose response headers carry the account's 5-hour and weekly windows. An account whose plan has no 5-hour window shows only the weekly one, never a 0% session line. A probe costs a little of that account's quota, so agx runs one only when you ask. Use this page to see how full a login is, to let agx choose the login with the most headroom for a run, and to know what each of those costs.

## Before you start

- At least one `oauth-token` login (`agx login ls`). An `api-key` login has no plan windows and is never probed.
- A network connection to the vendor API. The commands on this page that probe were not run for this guide (there are no real accounts behind it); the outputs under [Try it](#try-it) are the refusals that happen before any request.

## Step 1 — Know what a probe costs

One probe is one request to the Messages API with the smallest current model, a one-word message and a one-token reply (`provider/claude/probe.go`). It spends a few tokens of that login's plan quota and counts toward its rate limits. These commands probe, and nothing else does:

| Command | Probes |
|---|---|
| `agx usage --logins` | every login, after the home rows |
| `agx usage --login NAME` | that login only |
| `agx usage --logins --table`, `--login a,b`, `--login all` | every login listed, one table row each |
| `agx usage --pick` | every login you mark in the checklist |
| `agx login check [NAME]` | each login checked (`--no-probe` resolves only) |
| `--login auto` on `run`, `new`, `resume`, `ws` | every login that fits the profile |

A plain `agx`, `agx usage`, `agx login add` and `agx login ls` never probe (the test `TestLoginUsage_NeverProbesUnasked` in `internal/cli/loginusage_test.go` pins that).

## Step 2 — See every login's usage

```sh
agx usage --logins
```

The home rows come first, then one row per login. In `--json` a probed row carries `login`, `account` and `"source": "probe"` instead of `profile` and `home`, so a script can tell the two apart:

```sh
agx usage --logins --json | jq -r '.data[] | select(.source == "probe") | .login'
```

## Step 3 — See one login

```sh
agx usage --login personal
```

`--login home` is the plain home view. `--login NAME` cannot be combined with `--logins` or with profile names. A comma list or `all` prints the [login table](#step-4--compare-logins-in-one-table) instead; `home` and `auto` cannot be part of a list.

## Step 4 — Compare logins in one table

```sh
agx usage --logins --table        # every token login, one row each
agx usage --login amank,erman     # only these (a comma list prints the table without --table)
agx usage --login all             # every token login, same as --logins --table
agx usage --pick                  # mark logins in a checklist, then the table
agx usage --login all --sort day,-left --tz Asia/Kolkata   # by weekly reset day, most left first
```

Each row is one probe, all run at once with the per-account `--timeout`. Homes are not shown: the table is for token logins. An example with four logins (accounts are placeholders):

```text
#  LOGIN     PROVIDER  ACCOUNT        5H USED  5H RESETS    WEEKLY USED  WEEKLY RESETS  RESET DAY  RESETS ON         LEFT  STATUS
1  khanakia  claude    —              —        —            —            —              —          —                 —     rejected: blocked by org
2  erman     claude    e@example.com  0%       1 hr 0 min   98%          2 d 19 hr      Sat        Sat 7 Nov 07:00   2%    ok
5  ismail    claude    —              100%     1 hr 41 min  53%          3 d 18 hr      Sun        Sun 8 Nov 06:00   0%    limit reached
7  amank     claude    —              —        —            2%           6 d 22 hr      Wed        Wed 11 Nov 10:00  98%   ok
Weekly resets: Sat 1 (+98%) · Sun 1 (+53%) · Wed 1 (+2%)
  khanakia: login rejected by the server (HTTP 403: OAuth authentication is currently not allowed for this organization.)
```

- **#** is the login's position in `agx login ls`, so a row keeps its number whatever subset you show.
- **USED / RESETS** come in a pair per window: the 5-hour window, the weekly window, and a per-model weekly window (for example `FABLE WEEKLY`) only when some probe returned one. A window the account does not have is `—` in both cells, never `0%`: an account whose plan has no 5-hour window is not an idle one.
- **RESET DAY / RESETS ON** are the weekday and date the weekly window resets, in the `--tz` zone (default: the machine's local zone); `—` when the row has no weekly reset. The line under the table sums them up per weekday: how many logins reset then and the weekly USED they get back, soonest reset first.
- **--sort** orders the rows by comma-separated keys: `login`, `left`, `5h`, `5h-reset`, `weekly`, `weekly-reset`, `day` (Mon..Sun), `status`. A `-` prefix sorts high to low (`--sort day,-left`: by reset day, most left first within a day); a row without the value goes last either way, ties keep `agx login ls` order, and `#` stays the login's own number. `--sort` works only with the table.
- **LEFT** is 100 minus the fullest window, because any one window at its cap blocks the account. It is amber under 20% and red at 0%, and `—` when the probe returned no windows.
- **STATUS** is `ok`, `limit reached`, `rejected` (the server refused the token; `blocked by org` when the organisation disallows this kind of login, else the HTTP status), `expired`, `per token` (an api-key login, never probed) or `error` with a short reason (`timeout`, `rate-limited`, `inference-only token`, `not logged in`, or the start of the message). The full message of every failed row is printed under the table.
- The command exits 1 when any row failed, like the rest of `agx usage`.

In the checklist, space marks a login, `a` marks all (again: none), Enter shows the marked ones (or the one under the cursor when none is marked), and Esc or `q` cancels with exit 1. `--pick` needs a terminal on stdin and stdout; in a script, name the logins with `--login a,b`.

`--json` keeps the `usage.list` envelope and gives every row, home or login, with or without `--table`, the same verdict: `status`, `status_reason`, `left_percent` and `left_resets_at` (the last two absent when there are no windows), plus `weekly_resets_at` (RFC3339 in the `--tz` zone) and `weekly_reset_day` when the row has a weekly window:

```sh
agx usage --login all --json | jq -r '.data[] | "\(.login) \(.status) \(.left_percent // "-")"'
```

## Step 5 — Check a login before relying on it

```sh
agx login check personal
```

This resolves the token from its source and then probes. To confirm only that the token resolves, with no request:

```sh
agx login check personal --no-probe
```

## Step 6 — Let agx pick the login for a run

```sh
agx run -p work --login auto
```

agx fetches the home's own usage (free) and probes every login of the profile's provider that fits the profile, concurrently, then applies the same rule as `-p auto`: the lowest "fullest window" wins; ties go to the home, then to config order (`internal/cli/loginusage.go`, `autoLogin`; `pick/pick.go`). A login whose token does not resolve or whose probe fails is left out with its reason; if nothing is left, the run stops. The decision is printed on stderr in the form `agx: login auto → NAME (max N%) over OTHER (max N%), …` before the usual `agx: work · login NAME` line.

## Try it

These refusals happen before any network request (throwaway home with one login, `ci`):

```text
$ agx usage --login auto
agx: --login auto chooses a login for a launch; to compare every login use `agx usage --logins`

$ agx usage --login ci --logins
agx: --login ci shows one login; it cannot be combined with --logins or profile names

$ agx plan --login personal
agx: plan cannot show a token login: the profile endpoint refuses setup-tokens; use --login home or no --login (see a login's usage with `agx usage --login NAME`)
```

```text
$ agx usage --table
agx: --table shows token logins: add --logins, --login a,b (or all) or --pick

$ agx usage --pick --logins
agx: choose the logins one way: --logins, --login or --pick

$ agx usage --login ci,home
agx: --login home cannot be part of a list; list token login names or "all"

$ agx usage --pick < /dev/null
agx: --pick needs an interactive terminal (stdin and stdout); name the logins instead: --login a,b or --login all
```

Each exits 2.

## Pitfalls

- **Probing in a loop adds up.** Each probe is a real request on that account. A status bar that runs `agx usage --logins` every few seconds spends quota and rate limit on every account it lists; use plain `agx usage` (homes only, free) for that, and probe on demand. The table is no cheaper: `--login all` and `--pick` probe every login they show.
- **`auto` is only a flag.** It is refused in `AGX_LOGIN`, in a profile's or provider's `login:`, by `agx login use` and by `agx usage --login` (`config/logins.go`, `ErrAutoLogin`; `internal/cli/usage.go`, `usageLogin`), so probing never becomes a silent default.
- **A probed login has no account email.** The profile endpoint refuses setup-tokens (HTTP 403 with `error_code` `oauth_scope_insufficient`, which agx reports as `this token can only run Claude: it has no profile scope …`), so a probed row knows only that the token works; the `account` shown is the label you gave with `--account` (`provider/claude/login.go`, `LoginUsage`).
- **An api-key login is reported as billed per token, never probed.** It has no plan windows to compare, and `--login auto` treats it like any api-billed candidate, which auto-pick never chooses.
- **An expired stored login drops out of auto.** Past its expiry it fails to resolve (see [Add and manage logins](add-and-manage.md#step-8--renew-before-expiry)), so `--login auto` leaves it out with that reason rather than probing it.

## Related

- [Logins overview](overview.md)
- [Choosing a login](choosing-a-login.md)
- [Add and manage logins](add-and-manage.md)
