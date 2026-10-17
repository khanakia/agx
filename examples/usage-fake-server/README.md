# Usage against a fake server

Reads Claude Code plan usage through `claude.Provider.Usage` (the login stored in a home) and `claude.Provider.LoginUsage` (a raw token, read by one probe request), both against a local `httptest` server, then shows how HTTP 429 and 401 come back as `provider.ErrRateLimited` and `provider.ErrUnauthorized`. It writes a throwaway home with a fake `.credentials.json` and sets `Store: nil`, so neither the keychain nor the network is touched.

## Run

From the repository root:

```sh
go run ./examples/usage-fake-server
```

## Output

Captured from a real run:

<!-- out: usage-fake-server -->

```text
home login: logged in=true plan="Max (20x)"
  session Current session    5.0% normal   resets 2026-09-24T09:29:59Z
  weekly  All models        76.0% warning  resets 2026-09-24T17:59:59Z (binding)
  weekly  Fable             68.0% normal   resets 2026-09-24T17:59:59Z
  max: 76%
token login (probe):
  session Current session   12.0% normal   resets 2026-09-24T09:29:59Z
  weekly  All models        81.0% warning  resets 2026-09-29T17:59:59Z (binding)
  max: 81%
429: usage endpoint rate-limited this account; try again shortly (retry after 30s) (rate-limited=true)
bad token: login rejected by the server (HTTP 401) (unauthorized=true)
```

<!-- /out -->

## What to look at

- `UsageClient.Endpoint` and `ProbeClient.Endpoint` are the only seams needed to point the provider at a test server.
- The usage endpoint and the probe yield the same window model: the probe's `anthropic-ratelimit-unified-*` headers become the same "Current session" and "All models" rows, and `representative-claim` marks the binding one.
- `Usage.MaxPercent` is the figure auto-pick ranks by.
- Errors wrap the shared sentinels, so `errors.Is` works the same for every provider.

## Related

- Guide: [claude-and-codex.md](../../docs/guides/library/claude-and-codex.md)
- Source: [main.go](main.go)
- Test: [examples_test.go](../examples_test.go) builds and runs this program and checks the lines above.
