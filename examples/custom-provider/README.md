# A custom provider

Implements `provider.Provider` (plus `HistoryChecker`) for a made-up vendor CLI and plugs it into the same pieces agx uses for Claude Code and Codex: `config.Resolve` discovers its homes, `pick.Best` chooses an account, and `Launch` builds the command.

## Run

From the repository root:

```sh
go run ./examples/custom-provider
```

## Output

Captured from a real run:

<!-- out: custom-provider -->

```text
echo   default=true  max= 90% err=<nil>
spare  default=false max=  0% err=not logged in
work   default=false max= 35% err=<nil>
auto → work (max 35%) over echo (max 90%)
argv: echoagent --fast
env:  PATH=/usr/bin ECHOAGENT_HOME=~/.echoagent-work
```

<!-- /out -->

## What to look at

- Discovery, usage and launch are the only required behaviour; everything else is an optional upgrade interface.
- Returning the shared sentinels (`provider.ErrNotLoggedIn`, `provider.ErrAPIBilled`) is what makes pick and the renderers treat the new provider like a built-in one.
- `provider.HomeEnv` gives the new provider the same "remove the variable for the default home" rule.

## Related

- Guide: [provider-model.md](../../docs/guides/library/provider-model.md)
- Source: [main.go](main.go)
- Test: [examples_test.go](../examples_test.go) builds and runs this program and checks the lines above.
