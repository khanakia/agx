# Use agx as a Go library

Everything outside `internal/` in the agx module is a public, importable Go library: the vendor-neutral model of providers, profiles, usage and conversations, the Claude Code and Codex implementations, config loading and discovery, the auto-pick policy, session folders, secret references and named logins. You want these pages when you are writing your own tool around Claude Code or Codex (a status bar, a dashboard, a launcher, a different policy for choosing an account) and want agx's tested pieces without its CLI.

## Before you start

- Go 1.26.4 or newer (the module's `go` line), and the module on your import path:

```sh
go get github.com/khanakia/agx@latest
```

- Every page below quotes a runnable program from [`examples/`](../../../examples/). Clone the repository to run them; none of them needs an account, a token or the network.

## Step 1 — Pick the package you need

| Import path | What it gives you | Guide | Example |
|---|---|---|---|
| `github.com/khanakia/agx/provider` | The domain model: `Profile`, `Usage`, `Window`, `Conversation`, `Command`, the `Provider` interface and its optional upgrades, env helpers (`HomeEnv`, `SetEnv`, `UnsetEnv`, `ApplyLoginEnv`), `RateLimitError`, move plans (`MovePlan`, `ApplyMovePlan`, `CopyTree`) | [Provider model](provider-model.md) | [custom-provider](../../../examples/custom-provider/), [launch-command](../../../examples/launch-command/) |
| `github.com/khanakia/agx/provider/claude` | Claude Code: discovery, credentials (file and Keychain), usage, subscription, probe, conversations, `EncodePath`, launch, history moves, logins | [Claude Code and Codex](claude-and-codex.md) | [usage-fake-server](../../../examples/usage-fake-server/), [conversations](../../../examples/conversations/), [launch-command](../../../examples/launch-command/) |
| `github.com/khanakia/agx/provider/codex` | Codex: discovery, usage, subscription, conversations, launch | [Claude Code and Codex](claude-and-codex.md) | [launch-command](../../../examples/launch-command/), [config-resolve](../../../examples/config-resolve/) |
| `github.com/khanakia/agx/config` | Decode, load and resolve `config.yaml` against any providers; the `Editor` for comment-preserving edits | [Config, pick and sessions](config-pick-sessions.md) | [config-resolve](../../../examples/config-resolve/) |
| `github.com/khanakia/agx/pick` | The auto-pick policy: `Best` and `Result.Explain` | [Config, pick and sessions](config-pick-sessions.md) | [autopick](../../../examples/autopick/) |
| `github.com/khanakia/agx/sessions` | Session folders: `Slug`, `Name`, `Create`, `List`, `PlanGC`, `RemoveEmpty`, `Promote` | [Config, pick and sessions](config-pick-sessions.md) | [sessions-gc](../../../examples/sessions-gc/) |
| `github.com/khanakia/agx/secret` | `scheme:ref` secret references: `Parse`, `System.Resolve`, `ResolveAll`, the `Resolver` interface | [Secrets and logins](secrets-and-logins.md) | [secret-refs](../../../examples/secret-refs/) |
| `github.com/khanakia/agx/login` | Named logins: `Resolver`, `TokenFile`, `Clean`, `ExpiresAt`, `Owned`, `Compatible` | [Secrets and logins](secrets-and-logins.md) | [login-resolve](../../../examples/login-resolve/) |

## Step 2 — Respect the layering

The packages form layers, and a test enforces them so a convenient import cannot quietly couple them. `provider` imports only the standard library, so anything can depend on it. The vendor packages, `config`, `secret`, `login`, `pick` and `sessions` never import `internal/`, cobra or voltkit. Only `internal/cli` touches the command framework. A tool of yours therefore pulls in the standard library, `gopkg.in/yaml.v3` (for `config` and `login`) and nothing from agx's CLI.

<!-- snip: internal/archtest/archtest_test.go func:TestLayering -->

Source: [`internal/archtest/archtest_test.go`](../../../internal/archtest/archtest_test.go), `TestLayering`, quoted verbatim.

```go
func TestLayering(t *testing.T) {
	t.Parallel()
	pkgs := packageImports(t, repoRoot(t))
	if len(pkgs) < 8 {
		t.Fatalf("walked only %d packages — the guard is not looking at the repo", len(pkgs))
	}
	for dir, imps := range pkgs {
		for imp := range imps {
			// 1. The domain model imports only the standard library.
			if dir == "provider" && !isStdlib(imp) {
				t.Errorf("provider imports %s; it must stay stdlib-only", imp)
			}
			// 2. Libraries never import internal packages or the framework.
			isLibrary := strings.HasPrefix(dir, "provider") || dir == "config" || dir == "secret" || dir == "login" || dir == "pick" || dir == "sessions"
			if isLibrary && (strings.HasPrefix(imp, module+"/internal/") || imp == cobraPath || strings.HasPrefix(imp, "github.com/ubgo/voltkit/")) {
				t.Errorf("library %s imports %s", dir, imp)
			}
			// 3. Only internal/cli touches cobra and voltkit command modules.
			if (imp == cobraPath || imp == versioncmdPath) && dir != filepath.Join("internal", "cli") {
				t.Errorf("%s imports %s; only internal/cli may", dir, imp)
			}
			// 4. main wires the process and nothing else.
			if dir == "." && !isStdlib(imp) && imp != module+"/internal/cli" && imp != module+"/internal/appmeta" {
				t.Errorf("main imports %s; it may only import internal/cli and internal/appmeta", imp)
			}
		}
	}
}
```

<!-- /snip -->

## Step 3 — Run the examples

Each example is a `main` package in its own folder with a README that shows its real output.

```sh
go run ./examples/autopick
go run ./examples/config-resolve
go run ./examples/conversations
go run ./examples/custom-provider
go run ./examples/launch-command
go run ./examples/login-resolve
go run ./examples/secret-refs
go run ./examples/sessions-gc
go run ./examples/usage-fake-server
```

## Try it

```sh
go run ./examples/autopick
```

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

## Pitfalls

- The examples are tested, not just compiled: [`examples/examples_test.go`](../../../examples/examples_test.go) builds and runs every program and fails when an expected line is missing or a fake token appears in the output. A new example folder without expected lines in its `want` map fails the test too.
- `claude.New` reads the macOS Keychain. In tests and on other systems build the struct yourself with `Store: nil` (file credentials only), as every example does (`provider/claude/claude.go`, `New`).
- The usage endpoints rate-limit when polled repeatedly (`provider.ErrRateLimited`, `provider/provider.go`). Point `UsageClient.Endpoint` at an `httptest` server while developing instead of the real API.

## Related

- [Provider model](provider-model.md)
- [Claude Code and Codex](claude-and-codex.md)
- [Config, pick and sessions](config-pick-sessions.md)
- [Secrets and logins](secrets-and-logins.md)
- [Examples index](../../../examples/README.md)
- [Feature reference](../../reference/features.md#go-library-packages)
