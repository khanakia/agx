# Auto-pick the account with the most headroom

`-p auto` asks agx to choose the account for you: it fetches the plan usage of every subscription account of one provider, scores each one, and launches on the account that is furthest from any limit. Use it when you have two or more Claude (or Codex) subscriptions and want the next session to land on whichever still has room, without reading `agx usage` first.

## Before you start

- At least two plan-billed profiles of the same provider, each logged in once (run `agx run -p <name>` and log in inside the agent). `agx usage` should show windows, not `error: not logged in`, for each.
- The examples use the config from [Start an agent with `agx run`](run.md#before-you-start): `personal` and `work` are Claude subscriptions, `kimi` is api-billed, `codex` is the only Codex account.
- Every `-p auto` makes one usage request per account. The usage endpoints rate-limit callers that poll them in a loop (HTTP 429), so do not wrap `-p auto` in a retry loop.

## Step 1 — Launch with `-p auto`

`auto` works wherever a launch takes `-p`: `run` and `new`.

```sh
agx run -p auto
agx new -p auto invoice bug
```

For Codex accounts, add the provider; without it, auto-pick looks only at Claude accounts:

```sh
agx run -p auto --provider codex
```

## Step 2 — Read the decision

agx prints one line on stderr before the agent starts, naming the winner and every other eligible account with its score:

```text
agx: auto → work (max 5%) over personal (max 77%)
```

## Step 3 — Know how the score works

The policy lives in the `pick` package and has no I/O of its own:

1. **Candidates.** One per distinct home of the chosen provider whose profile is plan-billed. Two profiles that share a home (`personal` and `kimi` on `~/.claude`) are one candidate, represented by the plan-billed profile.
2. **Ineligible.** An account whose usage could not be fetched (not logged in, login expired, rate-limited, timed out after 15 seconds) is skipped with its reason. An api-billed profile is never a candidate.
3. **Score.** The highest percent across all of the account's windows: the session window, the weekly windows, any per-model weekly window, and the "Extra usage" row when it exists. Any single window at its cap blocks work, so the worst window is the one that counts.
4. **Winner.** The lowest score wins.
5. **Ties.** Equal scores keep config order: the profile listed first in `~/.agx/config.yaml` wins.

## Step 4 — Steer ties with config order

Put the account you would rather spend first at the top of `profiles`. With this file, `work` wins whenever both accounts score the same:

```yaml
profiles:
  - name: work
    provider: claude
    home: ~/.claude-work
  - name: personal
    provider: claude
    home: ~/.claude
    default: true
```

`default: true` does not affect auto-pick; it only decides which profile a plain `agx run` uses.

## Step 5 — Use the same policy from Go

The policy is a public package, so a script or another tool can make the same decision. This program builds three sets of candidates by hand and prints what `pick.Best` decides; it compiles in any module that requires `github.com/khanakia/agx`:

```go
package main

import (
	"errors"
	"fmt"

	"github.com/khanakia/agx/pick"
	"github.com/khanakia/agx/provider"
)

// usage builds a Usage with one window per percent.
func usage(pcts ...float64) provider.Usage {
	var u provider.Usage
	for _, p := range pcts {
		u.Windows = append(u.Windows, provider.Window{Percent: p})
	}
	return u
}

func main() {
	plan := provider.BillingPlan
	cases := [][]pick.Candidate{
		{
			{Profile: provider.Profile{Name: "personal", Billing: plan}, Usage: usage(12, 77)},
			{Profile: provider.Profile{Name: "work", Billing: plan}, Usage: usage(5, 3)},
		},
		{
			{Profile: provider.Profile{Name: "personal", Billing: plan}, Usage: usage(40, 10)},
			{Profile: provider.Profile{Name: "work", Billing: plan}, Usage: usage(40, 2)},
			{Profile: provider.Profile{Name: "team", Billing: plan}, Err: errors.New("not logged in")},
		},
		{
			{Profile: provider.Profile{Name: "personal", Billing: plan}, Usage: usage(100, 20)},
			{Profile: provider.Profile{Name: "work", Billing: plan}, Usage: usage(64)},
		},
	}
	for _, c := range cases {
		res, err := pick.Best(c)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Println(res.Explain())
		for name, why := range res.Skipped {
			fmt.Println("  skipped", name+":", why)
		}
	}
}
```

The second case is the tie rule at work: both accounts score 40% (their worst window), so `personal`, listed first, wins even though its other window is fuller.

## Try it

The Go program above prints:

```text
auto → work (max 5%) over personal (max 77%)
auto → personal (max 40%) over work (max 40%)
  skipped team: not logged in
auto → work (max 64%) over personal (max 100%)
```

On a machine where no account is logged in, auto-pick has nothing to choose from and says why for each account:

```text
$ agx run -p auto --dry-run
agx: pick: no eligible profile: personal: not logged in — run `agx run -p personal` once to log in / refresh; work: not logged in — run `agx run -p work` once to log in / refresh

$ echo $?
1
```

## Pitfalls

- **`--dry-run` still fetches usage.** The profile is resolved before the dry run is printed, so `agx run -p auto --dry-run` makes the same usage requests as a real launch (`internal/cli/launch.go`: `resolveProfile` runs first, then `launch`).
- **There is no threshold.** If every account is at 100%, auto-pick still launches on the lowest score (or the first one in config order); it never refuses because all accounts are full (`pick/pick.go`, `Best`). Check `agx usage` when every account is close to its cap.
- **Api-billed profiles are never picked**, even when they would be the only option (`pick/pick.go`; tested by `TestBest_APIBilledNeverChosen` in `pick/pick_test.go`). Launch them by name: `agx run -p kimi`.
- **No eligible account is exit 1**, and the message lists each account's reason in config order (`pick.ErrNoCandidate`; tested by `TestBest_NoCandidate`).
- **`-p auto` is not the same as `--login auto`.** `-p auto` chooses the home (history and settings move with it); `--login auto` keeps the home and chooses whose quota to spend by probing each login. See [Logins](../logins/usage-and-auto.md).
- **`resume` does not take `-p auto`.** `resume` already launches on the account that stores the conversation; `agx resume -p auto` treats `auto` as a profile name and fails with `no profile uses <home> (claude); add one to the config` (`internal/cli/launch.go`, `resumeConversation`).

## Related

- [Start an agent with `agx run`](run.md)
- [Usage and plans](../usage/usage-and-plans.md) for the windows the score is computed from
- [Logins: usage and auto](../logins/usage-and-auto.md)
- [README: Choosing a profile](../../../README.md#choosing-a-profile--p)
