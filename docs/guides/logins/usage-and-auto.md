# Login usage and `--login auto`

A home's usage comes from a free endpoint, but a setup-token cannot read that endpoint (it answers 403). So agx reads a login's usage with a **probe**: one minimal request whose response headers carry the account's 5-hour and weekly windows. A probe costs a little of that account's quota, so agx runs one only when you ask. Use this page to see how full a login is, to let agx choose the login with the most headroom for a run, and to know what each of those costs.

## Before you start

- At least one `oauth-token` login (`agx login ls`). An `api-key` login has no plan windows and is never probed.
- A network connection to the vendor API. The commands on this page that probe were not run for this guide (there are no real accounts behind it); the outputs under [Try it](#try-it) are the refusals that happen before any request.

## Step 1 — Know what a probe costs

One probe is one request to the Messages API with the smallest current model, a one-word message and a one-token reply (`provider/claude/probe.go`). It spends a few tokens of that login's plan quota and counts toward its rate limits. These commands probe, and nothing else does:

| Command | Probes |
|---|---|
| `agx usage --logins` | every login, after the home rows |
| `agx usage --login NAME` | that login only |
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

`--login home` is the plain home view. `--login NAME` cannot be combined with `--logins` or with profile names.

## Step 4 — Check a login before relying on it

```sh
agx login check personal
```

This resolves the token from its source and then probes. To confirm only that the token resolves, with no request:

```sh
agx login check personal --no-probe
```

## Step 5 — Let agx pick the login for a run

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

Each exits 2.

## Pitfalls

- **Probing in a loop adds up.** Each probe is a real request on that account. A status bar that runs `agx usage --logins` every few seconds spends quota and rate limit on every account it lists; use plain `agx usage` (homes only, free) for that, and probe on demand.
- **`auto` is only a flag.** It is refused in `AGX_LOGIN`, in a profile's or provider's `login:`, by `agx login use` and by `agx usage --login` (`config/logins.go`, `ErrAutoLogin`; `internal/cli/usage.go`, `usageLogin`), so probing never becomes a silent default.
- **A probed login has no account email.** The profile endpoint refuses setup-tokens, so a probed row knows only that the token works; the `account` shown is the label you gave with `--account` (`provider/claude/login.go`, `LoginUsage`).
- **An api-key login is reported as billed per token, never probed.** It has no plan windows to compare, and `--login auto` treats it like any api-billed candidate, which auto-pick never chooses.
- **An expired stored login drops out of auto.** Past its expiry it fails to resolve (see [Add and manage logins](add-and-manage.md#step-8--renew-before-expiry)), so `--login auto` leaves it out with that reason rather than probing it.

## Related

- [Logins overview](overview.md)
- [Choosing a login](choosing-a-login.md)
- [Add and manage logins](add-and-manage.md)
