# Launch commands

Builds the `provider.Command` agx would exec for Claude Code and Codex and prints it instead of running it. It shows the env rule every wrapper has to copy, and what a token login adds and removes.

## Run

From the repository root:

```sh
go run ./examples/launch-command
```

## Output

Captured from a real run:

<!-- out: launch-command -->

```text
personal (default home):
  argv: claude
  dir:  /srv/demo/api
  env:  ANTHROPIC_API_KEY=<set>
work, resuming a1:
  argv: claude --model opus --resume a1 --verbose
  env:  ANTHROPIC_API_KEY=<set> CLAUDE_CONFIG_DIR=/home/demo/.claude-work
work on a token login:
  argv: claude --model opus --resume a1 --verbose
  env:  CLAUDE_CODE_OAUTH_TOKEN=<set> CLAUDE_CONFIG_DIR=/home/demo/.claude-work
codex, resuming:
  argv: codex resume 019a-demo
  env:  ANTHROPIC_API_KEY=<set> CLAUDE_CONFIG_DIR=/home/demo/.claude-old CODEX_HOME=/home/demo/.codex-team
```

<!-- /out -->

## What to look at

- The default home runs with `CLAUDE_CONFIG_DIR` removed, even though the base env exported a stale value.
- A non-default home sets `CLAUDE_CONFIG_DIR`; profile args come before `--resume` and per-call args.
- `LoginEnv` + `provider.ApplyLoginEnv` set `CLAUDE_CODE_OAUTH_TOKEN` and remove `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN`.
- `Launch` alone does not strip credential variables: `ANTHROPIC_API_KEY` from the base env survives until a login removes it, and Codex passes Claude's variables through untouched.

## Related

- Guide: [claude-and-codex.md](../../docs/guides/library/claude-and-codex.md)
- Source: [main.go](main.go)
- Test: [examples_test.go](../examples_test.go) builds and runs this program and checks the lines above.
