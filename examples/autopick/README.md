# Auto-pick policy

Runs `pick.Best` over four candidates and prints `Result.Explain`, the one line agx writes to stderr for `run -p auto`. It also shows a tie and the error when no candidate is eligible. The package is pure, so the usage figures are written inline.

## Run

From the repository root:

```sh
go run ./examples/autopick
```

## Output

Captured from a real run:

<!-- out: autopick -->

```text
auto → work (max 12%) over personal (max 77%)
chosen=work score=12
skipped kimi: billed per token: no usage windows
skipped spare: not logged in
auto → a (max 40%) over b (max 40%)
none eligible: pick: no eligible profile: kimi: billed per token: no usage windows; spare: not logged in (no-candidate=true)
```

<!-- /out -->

## What to look at

- A profile's score is its busiest window, not its average: `personal` has a nearly free session window but scores 77.
- `billing: api` profiles and candidates whose fetch failed are skipped with a reason in `Result.Skipped`.
- Ties keep candidate order (config order).
- With nobody eligible the error wraps `pick.ErrNoCandidate` and lists every reason.

## Related

- Guide: [config-pick-sessions.md](../../docs/guides/library/config-pick-sessions.md)
- Source: [main.go](main.go)
- Test: [examples_test.go](../examples_test.go) builds and runs this program and checks the lines above.
