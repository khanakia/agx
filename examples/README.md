# Examples

Runnable programs that use agx's Go library, one per folder. Each uses only temporary directories and local `httptest` servers: no real home, account, token or network. Each folder's README shows the program's real output, and [`examples_test.go`](examples_test.go) builds and runs every program and checks that output, so the examples cannot drift from the library.

| Example | Shows | Guide |
|---|---|---|
| [autopick](autopick/) | `pick.Best` and `Result.Explain`: scoring, skips, ties, no candidate | [Config, pick and sessions](../docs/guides/library/config-pick-sessions.md) |
| [config-resolve](config-resolve/) | `config.Load` / `Decode` / `Resolve` with discovery in a fake home, strict decoding, alias validation | [Config, pick and sessions](../docs/guides/library/config-pick-sessions.md) |
| [conversations](conversations/) | `claude.Provider.Conversations`, `EncodePath`, `HasHistory`, `MoveHistory`, `PlanMove` + `provider.ApplyMovePlan` | [Claude Code and Codex](../docs/guides/library/claude-and-codex.md) |
| [custom-provider](custom-provider/) | Implementing `provider.Provider` for another CLI and using it with config and pick | [Provider model](../docs/guides/library/provider-model.md) |
| [launch-command](launch-command/) | `Launch` for Claude Code and Codex, the home variable rule, a token login's env | [Claude Code and Codex](../docs/guides/library/claude-and-codex.md) |
| [login-resolve](login-resolve/) | `login.Resolver`, `TokenFile`, `ExpiresAt`, `Compatible` and the refusals | [Secrets and logins](../docs/guides/library/secrets-and-logins.md) |
| [secret-refs](secret-refs/) | `secret.Parse`, `System`, `ResolveAll`, a custom `Resolver` | [Secrets and logins](../docs/guides/library/secrets-and-logins.md) |
| [sessions-gc](sessions-gc/) | `sessions.Create`, `List`, `PlanGC`, `RemoveEmpty`, `Promote` | [Config, pick and sessions](../docs/guides/library/config-pick-sessions.md) |
| [usage-fake-server](usage-fake-server/) | `claude.Provider.Usage` and the probe against an `httptest` server, 429 and 401 errors | [Claude Code and Codex](../docs/guides/library/claude-and-codex.md) |

Run one from the repository root:

```sh
go run ./examples/autopick
```

Run them all as a test:

```sh
go test ./examples/
```
