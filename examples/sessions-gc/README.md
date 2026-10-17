# Session folders and gc

Creates session folders the way `agx new` names them, lists them, plans garbage collection with `sessions.PlanGC`, removes what the plan allows with `RemoveEmpty`, and promotes one folder with `Promote`. History is answered by a callback, so the program decides which folder "has history".

## Run

From the repository root:

```sh
go run ./examples/sessions-gc
```

## Output

Captured from a real run:

<!-- out: sessions-gc -->

```text
same-second twin: chat_20261003_090000_2
  20261006_080000                  files=0 remove=false newer than min age
  chat_20261003_090000             files=0 remove=false has conversation history
  chat_20261003_090000_2           files=0 remove=true  empty, no history
  notes_20261002_090000            files=1 remove=false has files
  try_the_new_api_20261001_090000  files=0 remove=true  empty, no history
removed chat_20261003_090000_2
removed try_the_new_api_20261001_090000
remove non-empty: refused=true
promoted notes_20261002_090000 → projects/notes-app
promote onto existing: exists=true
```

<!-- /out -->

## What to look at

- Folder names are `[slug_]YYYYMMDD_HHMMSS`; two creates in one second get a `_2` suffix.
- A folder is collected only when it has no files, no history, and is older than the minimum age (taken from the name's stamp).
- `RemoveEmpty` re-checks at delete time and refuses a folder that holds a file.
- `Promote` refuses an existing destination (`sessions.ErrExists`).

## Related

- Guide: [config-pick-sessions.md](../../docs/guides/library/config-pick-sessions.md)
- Source: [main.go](main.go)
- Test: [examples_test.go](../examples_test.go) builds and runs this program and checks the lines above.
