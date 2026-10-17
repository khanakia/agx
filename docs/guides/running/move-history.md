# Move a folder's history to another account: `agx sessions move`

`agx sessions move` moves every conversation that was started in one folder from one Claude account's home to another's, so `agx resume`, `claude -c` and `claude --resume` continue those chats on the new account. Use it when a project you began on your personal subscription now belongs on your work one, or to undo that later. It works between accounts of the same provider only, and it refuses Codex, for a reason explained below.

## Before you start

- The examples use the config from [Start an agent with `agx run`](run.md#before-you-start): `personal` (`~/.claude`) and `work` (`~/.claude-work`) are Claude accounts, `kimi` shares `~/.claude`, `codex` is a Codex account.
- The folder used below, `~/code/docker_setup_mac`, has three conversations on `personal`.
- Exit Claude Code in that folder first, in both accounts. A running session blocks the move.

## Step 1 — See the plan with `--dry-run`

```sh
cd ~/code/docker_setup_mac
agx sessions move --to work --dry-run
```

The target profile (`--to`, required) decides the provider: `work` is a Claude profile, so Claude conversations move. The sources are every other home of that provider, or only `--from` when you give it. The plan lists each conversation id and title and how many files and folders are involved, and changes nothing.

## Step 2 — Move

```sh
agx sessions move --to work
```

For each conversation, Claude's history folder `<home>/projects/<encoded folder path>/` (every character other than `A-Z`, `a-z`, `0-9` in the path replaced by `-`) contributes:

- the transcript `<id>.jsonl` and the tool-result folder `<id>/`,
- the file checkpoints used by rewind, `file-history/<id>/`,
- the environment snapshot, `session-env/<id>/`,
- and, on a full move, the folder's project memory, `projects/<encoded path>/memory/`.

The steps, in order:

1. **Plan and check.** The plan is computed without touching anything. It is blocked when Claude Code is running in the folder (in either home, read from Claude Code's own registry `<home>/sessions/<pid>.json`, only for processes that are still alive), or when any target path already exists. A blocked plan is printed with each reason and nothing changes.
2. **Back up.** Each source item is copied to `<source home>/backups/agx-move-<folder>-<YYYYMMDD-HHMMSS>/`, keeping its path relative to the home; when that folder already exists (a second move within the same second), the name gets `-2`, `-3` and so on. `--no-backup` skips this.
3. **Rename.** Each item is renamed into the target home, which merges into whatever the target already holds for that folder. If any rename fails, the items already renamed are renamed back, so both homes end as they were.
4. **Clean up.** After a full move, the now-empty history folder (and its empty `memory/`) in the source home is removed, so the old account no longer reports history for the folder. Anything the move does not recognise stays and is listed as `left in place`.

## Step 3 — Move only some conversations

```sh
agx sessions move . --to work --from personal
agx sessions move . --to work --only aeba
agx sessions move . --to work --only aeba9843,6144697a
agx sessions move ~/code/docker_setup_mac --to work
agx sessions move docker_setup_mac --to work
```

`--only` takes full ids or unique prefixes, comma-separated. A partial move leaves the folder's memory where it is, since other conversations stay behind. The folder argument defaults to the current folder; a bare name is looked up under `sessions.root`, then in the current folder, then matched against the current folder's own name, and anything with a `/` is a path.

## Step 4 — Pick chats instead of typing ids

```sh
agx sessions move -i
agx sessions move -i --to work --dry-run
agx sessions move -i --all
```

`-i` (`--pick`) lists this folder's chats; mark several with Tab in fzf, or type numbers such as `1 3` or `1,3` without fzf. When `--to` is missing, a second list offers every profile of the chats' provider and marks the ones that hold the chosen chats now. `--all` first opens the folder picker from [`agx ws`](workspace-picker.md). The chosen chats become `--only`, and every check above still applies. `-i` cannot be combined with a folder argument or `--only`.

## Step 5 — Undo

```sh
agx sessions move . --to personal --from work
```

Moving back is the same operation in the other direction. The backup folder still holds the untouched originals if you need to restore by hand.

## Try it

```text
$ agx sessions move --to work --dry-run
would move 3 claude conversation(s) for ~/code/docker_setup_mac
  from ~/.claude → ~/.claude-work (work)
    33f18428-fc35-4b31-baeb-8e9f01576ff9  (untitled)
    6144697a-4e8c-4ef3-8caa-f5c0498e8530  (untitled)
    aeba9843-dbea-4d03-a715-7f83e009bf7c  Hatchet, ClickHouse, HyperDX services
  6 file(s)/folder(s) involved

$ agx sessions move --to work
moved 3 claude conversation(s) for ~/code/docker_setup_mac
  from ~/.claude → ~/.claude-work (work)
    33f18428-fc35-4b31-baeb-8e9f01576ff9  (untitled)
    6144697a-4e8c-4ef3-8caa-f5c0498e8530  (untitled)
    aeba9843-dbea-4d03-a715-7f83e009bf7c  Hatchet, ClickHouse, HyperDX services
  6 file(s)/folder(s) involved
  backup: ~/.claude/backups/agx-move-docker_setup_mac-20261017-024917
done — continue with: cd ~/code/docker_setup_mac && agx resume  (or claude -c on work)

$ agx resume --list
1 d ago      claude work       Hatchet, ClickHouse, HyperDX se… ~/code/docker_setup_mac       aeba9843
1 d ago      claude work       (untitled)                       ~/code/docker_setup_mac       33f18428
1 d ago      claude work       (untitled)                       ~/code/docker_setup_mac       6144697a

$ agx sessions move --to work
no claude conversations for ~/code/docker_setup_mac outside work — nothing to move
```

A partial move lists what stays:

```text
$ agx sessions move --to work --only aeba --dry-run
would move 1 claude conversation(s) for ~/code/docker_setup_mac
  from ~/.claude → ~/.claude-work (work)
    aeba9843-dbea-4d03-a715-7f83e009bf7c  Hatchet, ClickHouse, HyperDX services
  3 file(s)/folder(s) involved
  · left in place: ~/.claude/projects/-home-you-code-docker-setup-mac/33f18428-fc35-4b31-baeb-8e9f01576ff9.jsonl
  · left in place: ~/.claude/projects/-home-you-code-docker-setup-mac/6144697a-4e8c-4ef3-8caa-f5c0498e8530.jsonl
```

Blocked moves print the plan, each reason, and exit 1:

```text
$ agx sessions move --to work
can't move 3 claude conversation(s) for ~/code/docker_setup_mac
  from ~/.claude → ~/.claude-work (work)
    33f18428-fc35-4b31-baeb-8e9f01576ff9  (untitled)
    6144697a-4e8c-4ef3-8caa-f5c0498e8530  (untitled)
    aeba9843-dbea-4d03-a715-7f83e009bf7c  Hatchet, ClickHouse, HyperDX services
  6 file(s)/folder(s) involved
  ✗ Claude is running in this folder (pid 2740010) — exit it first
agx: move blocked — see the reasons above; nothing was changed

$ agx sessions move --to work
can't move 3 claude conversation(s) for ~/code/docker_setup_mac
  from ~/.claude → ~/.claude-work (work)
    33f18428-fc35-4b31-baeb-8e9f01576ff9  (untitled)
    6144697a-4e8c-4ef3-8caa-f5c0498e8530  (untitled)
    aeba9843-dbea-4d03-a715-7f83e009bf7c  Hatchet, ClickHouse, HyperDX services
  6 file(s)/folder(s) involved
  ✗ /home/you/.claude-work/projects/-home-you-code-docker-setup-mac/aeba9843-dbea-4d03-a715-7f83e009bf7c.jsonl already exists in the target account
agx: move blocked — see the reasons above; nothing was changed
```

Requests that cannot work are refused before any plan is made:

```text
$ agx sessions move --from personal --to codex
agx: can't move conversations from personal (claude) to codex (codex): providers store conversations in different formats — choose a --to profile of the same provider

$ agx sessions move --from personal --to kimi
agx: personal and kimi both use ~/.claude: source and target use the same home, so there is nothing to move

$ agx sessions move --to codex
agx: moving codex conversations is not supported: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite), so moving its files would break its index; continue Codex work in the original account
```

Picking without fzf, in `~/code/app`: chats 1 and 4, then target 2:

```text
$ agx sessions move -i --dry-run
  1) 3 min ago    claude personal   Fix the login redirect loop      ~/code/app                    2bec3f80
  2) 40 min ago   claude work       Invoice export to CSV            ~/code/app                    dbcaeaf2
  3) 1 hr ago     codex  codex      (untitled)                       ~/code/app                    019eea1d
  4) 1 hr ago     claude personal   Add pagination to orders         ~/code/app                    5c9565db
move (tab: mark) [numbers 1-4, e.g. 1 3; Enter to cancel]: 1 4
  1) personal   claude ~/.claude                    ← 2 chat(s) here now
  2) work       claude ~/.claude-work
  3) kimi       claude ~/.claude                    ← 2 chat(s) here now
move to [1-3, Enter to cancel]: 2
would move 2 claude conversation(s) for ~/code/app
  from ~/.claude → ~/.claude-work (work)
    2bec3f80-6a1e-4c55-9d0e-3f1a7c2b9e11  Fix the login redirect loop
    5c9565db-0f4e-4b7a-8a21-9d3c6e4f1a22  Add pagination to orders
  2 file(s)/folder(s) involved
```

`--json` prints a `session.move` envelope with one entry per source home: `provider`, `from_home`, `to_home`, `dir`, `conversations`, `items` (each `from` and `to`), `blockers`, `left`, `titles`, `backup`, `dry_run` and `applied`.

## Exit codes

| Code | When |
|---|---|
| 0 | moved, or nothing to move (running it twice is harmless) |
| 1 | blocked (nothing changed), a failed backup or rename, Codex refused, an `--only` id not found |
| 2 | a wrong request: missing `--to`, unknown profile, different providers, same home, missing folder, `-i` with a folder or `--only` |

## Pitfalls

- **Same provider only.** Claude and Codex store conversations in different formats, so a move between them is refused with exit 2 (`internal/cli/move.go`, `errCrossProvider`; tested by `TestSessionsMove_Refusals` in `internal/cli/move_test.go`).
- **Codex is refused on purpose.** Codex keeps an index of its conversations in a SQLite database (`state_5.sqlite`, table `threads`) that stores each rollout file's absolute path, so renaming the files into another home would leave that index pointing at files that are gone. agx says so instead of trying (`provider/codex/codex.go`, `MoveUnsupportedReason`). Continue Codex work in the account that has it.
- **The target home matters, not the profile name.** `kimi` shares `~/.claude` with `personal`, so `--from personal --to kimi` has nothing to move (exit 2), while `agx sessions move --to kimi` without `--from` moves conversations from every other home, `~/.claude-work` included, into `~/.claude` (`internal/cli/move.go`, `moveSources`).
- **A running Claude blocks the move**, but only when its registry entry names a live process: stale entries from crashed sessions are ignored so they cannot block a move forever (`provider/claude/move.go`, `liveSessions`; tested by `TestPlanMove_Blockers` and `TestLiveSessions_PIDForms` in `provider/claude/move_test.go`).
- **A failed rename rolls back.** If a rename fails half-way, the items already moved are renamed back and the error ends in `(nothing was changed)`; only if the rollback also fails does the message point at the backup folder (`provider/move.go`, `ApplyMovePlan`; tested by `TestApplyMovePlan_RollsBackOnFailure` in `provider/move_test.go`). Renames need both homes on the same disk; across disks every move fails and rolls back.
- **Backup names have one-second resolution, plus a suffix.** A second move out of the same home for the same folder within the same second gets `agx-move-<folder>-<time>-2`, then `-3`, so two backups never share a folder (`internal/cli/move.go`, `uniqueBackupDir`).
- **An ambiguous `--only` prefix reads as "not found".** If two ids start with the prefix you typed, the message is `agx: claude: no conversation "6" for <folder> in <home>`, exit 1; type more characters (`provider/claude/move.go`, `matchID`).
- **Only the items listed in Step 2 move.** Claude Code's global prompt history and the folder's trust and allowed-tools settings are not part of the plan and stay in the old account, so expect Claude Code to ask you to trust the folder again on the new account (`provider/claude/move.go`, `PlanMove`).
- **Several source homes move one after another.** Without `--from`, each other home of the provider is planned and applied in turn; a rollback covers one home's items, so if a later home fails, the earlier homes have already moved. Use `--from` to move one home at a time when that matters (`internal/cli/move.go`, `runMove`).

## Related

- [Resume a conversation](resume.md): after a move, `agx resume` in the folder opens the chats on the new account
- [Session folders](new-and-sessions.md): `promote` moves a folder and re-keys its history within one account
- [Logins](../logins/overview.md): to spend another account's quota without moving any history
- [JSON and scripting](../usage/json-and-scripting.md) for the `session.move` envelope
- [README: Switch a project to another account](../../../README.md#switch-a-project-to-another-account-agx-sessions-move)
