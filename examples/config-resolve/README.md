# Config resolve and discovery

Resolves agx's YAML config against the Claude Code and Codex providers in a throwaway home: first with no file (every discovered home becomes a profile), then with a file that configures Claude profiles (Claude's discovered homes are dropped, Codex's stays). It ends with the two refusals config validation is strictest about: an unknown key and an alias value that is not shell-safe.

## Run

From the repository root:

```sh
go run ./examples/config-resolve
```

## Output

Captured from a real run:

<!-- out: config-resolve -->

```text
no file (found=false), sessions root ~/agx-sessions
  personal claude ~/.claude       plan default=true  discovered
  work     claude ~/.claude-work  plan default=false discovered
  codex    codex  ~/.codex        plan default=true  discovered
with file (found=true), sessions root ~/work/sessions, promote root ~/work
  main     claude ~/.claude-work  plan default=true  config
  kimi     claude ~/.claude-work  api  default=false config
  codex    codex  ~/.codex        plan default=true  discovered
resume in ~/.claude-work runs on: main (plan before api)
alias clm → agx run -p main
typo: config: invalid: yaml: unmarshal errors:
  line 1: field profile not found in type config.File (invalid=true)
unsafe alias: config: invalid: shell.aliases.bad: only letters, digits, spaces and _./:=@+- are allowed
```

<!-- /out -->

## What to look at

- `config.Load` treats a missing file as "no config", not an error; `Found` says which.
- `~/.claude-worktrees` holds no config marker, so discovery skips it.
- Once a provider has one configured profile, the config is the whole list for that provider.
- `Config.ForHome` prefers a plan-billed profile when several share a home.
- `sessions.promote_root` defaults to the parent of `sessions.root`.

## Related

- Guide: [config-pick-sessions.md](../../docs/guides/library/config-pick-sessions.md)
- Source: [main.go](main.go)
- Test: [examples_test.go](../examples_test.go) builds and runs this program and checks the lines above.
