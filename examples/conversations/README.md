# Conversations and history moves

Writes four synthetic Claude Code transcripts into a throwaway home, lists them with `Provider.Conversations` (all, and one folder with a limit), re-keys a moved folder's history with `MoveHistory`, then moves another folder's conversations into a second home with `PlanMove` and `provider.ApplyMovePlan` (with a backup).

## Run

From the repository root:

```sh
go run ./examples/conversations
```

## Output

Captured from a real run:

<!-- out: conversations -->

```text
history key for /srv/demo/api: -srv-demo-api
all conversations, newest first:
  w1  /srv/demo/web     "Add dark mode"
  a1  /srv/demo/api     "Login fix (final)"
  a2  /srv/demo/api     ""
newest in /srv/demo/api:
  a1  /srv/demo/api     "Login fix (final)"
has history: /srv/demo/api=true /srv/demo/none=false
moved=true; now listed under /srv/demo/api-v2:
  a1  /srv/demo/api-v2  "Login fix (final)"
  a2  /srv/demo/api-v2  ""
plan: 2 conversation(s), 2 item(s), 0 blocker(s)
  ~/.claude/projects/-srv-demo-web/w0.jsonl
    → ~/.claude-work/projects/-srv-demo-web/w0.jsonl
  ~/.claude/projects/-srv-demo-web/w1.jsonl
    → ~/.claude-work/projects/-srv-demo-web/w1.jsonl
after apply, ~/.claude-work holds 1 conversation(s) for /srv/demo/web
```

<!-- /out -->

## What to look at

- History lives under `<home>/projects/<EncodePath(dir)>`; a folder lookup is one directory read, not a scan.
- A user-set `custom-title` wins over the generated `ai-title`.
- A transcript with no `cwd` record is not listed, but `PlanMove` still moves it (2 conversations planned, 1 listed after the move).
- After `MoveHistory`, a `Dir` query reports the queried folder even though the transcript still records the old cwd.

## Related

- Guide: [claude-and-codex.md](../../docs/guides/library/claude-and-codex.md)
- Source: [main.go](main.go)
- Test: [examples_test.go](../examples_test.go) builds and runs this program and checks the lines above.
