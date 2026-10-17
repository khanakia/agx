# Session folders: `agx new` and `agx sessions`

`agx new` creates a fresh, timestamped folder under one sessions root and starts the agent inside it, so throwaway experiments never land in your home folder or in a real project. `agx sessions` then manages those folders: `ls` shows what each one holds, `gc` removes the empty ones nobody talked to, and `promote` turns a spike that grew into a real project, taking its Claude conversation history along. Use them when you start many short agent sessions and want them in one place you can clean up safely.

## Before you start

- The examples use the config from [Start an agent with `agx run`](run.md#before-you-start), whose `sessions` block is:

```yaml
sessions:
  root: ~/work/sessions
  promote_root: ~/work
```

- Without that block, `sessions.root` is `~/agx-sessions` and `sessions.promote_root` is the parent of the root. Both accept `~`.
- To have your shell end up inside the new folder after the agent exits, load the shell layer; see [Shell integration](../config-shell/shell-integration.md).

## Step 1 — Start in a new folder

`agx new` takes the same `-p`, `--provider`, `--login` and `--dry-run` flags as `agx run`. Any other words become the folder's slug; words after `--` are passed to the agent instead:

```sh
agx new
agx new pglite spike
agx new -p work "Fix: Login/Redirect!"
agx new -p auto
agx new -p work pglite -- --model opus
```

The folder is named `[slug_]YYYYMMDD_HHMMSS` in local time. The slug is your words lowercased, with every run of characters other than `a-z` and `0-9` turned into one `_` and leading or trailing `_` trimmed, so `Fix: Login/Redirect!` becomes `fix_login_redirect`. Two folders created in the same second get `_2`, `_3` and so on instead of sharing one.

## Step 2 — List the folders

```sh
agx sessions ls
agx sessions ls --json
```

Each row shows the folder, its last activity (the newest modification time of the folder or any file in it), how many files it holds (`.DS_Store` does not count) and whether any account has a conversation that was started in it. Newest folders come first, by the timestamp in the name, or by modification time for a folder you created by hand.

## Step 3 — Clean up with `gc`

`gc` without `--yes` only prints the plan:

```sh
agx sessions gc
agx sessions gc --yes
agx sessions gc --min-age 72h --yes
```

A folder is removed only when all three hold:

1. it has no files (`.DS_Store` ignored),
2. no account, Claude or Codex, has a conversation started in it,
3. it is older than `--min-age` (default `24h`), measured from the timestamp in its name, or from its modification time when the name has none.

The delete itself can only remove empty directories: `.DS_Store` files are removed first, then each directory bottom-up with a call that refuses a directory that still has content. A file written into a folder after the plan was made makes that folder fail instead of being deleted (`sessions/sessions.go`, `RemoveEmpty`). A failure is reported as a warning, the other folders are still removed, and the exit code is 1.

## Step 4 — Promote a folder into a project

```sh
agx sessions promote pglite_spike_20261010_101500 pglite-go --dry-run
agx sessions promote pglite_spike_20261010_101500 pglite-go
agx sessions promote . my-tool --git-init
agx sessions promote . my-tool --to ~/code
```

What happens, in order:

1. **The folder is found.** `.` is the folder you are in. A bare name is looked up under `sessions.root`, then in the current folder, then matched against the current folder's own name. Anything containing `/` is a path.
2. **The new name is checked.** It must be one folder name: `..`, `.` and anything with a `/` are refused.
3. **The destination is checked.** `<promote_root>/<name>`, or `<--to>/<name>`; an existing destination is refused, never merged.
4. **History is found.** Every account's home is asked whether it holds conversations started in the folder.
5. **The folder is moved** with one rename. Parent folders of the destination are created.
6. **Claude history follows.** Claude Code files a folder's conversations under `<home>/projects/<path with every character other than A-Z, a-z, 0-9 replaced by ->`. agx renames that history folder in every Claude home that has one, so `agx resume`, `claude -c` and `claude --resume` in the new folder find the same chats.
7. **Codex history stays.** Codex files conversations by date and id, not by folder, so nothing moves; agx prints a warning that resume still finds them but opens the old path.
8. **`git init`** runs in the new folder when you pass `--git-init`.

`--dry-run` stops after step 4 and prints the plan; `--json` prints a `session.promote` envelope.

## Try it

```text
$ agx new pglite spike --dry-run
profile: personal (claude, /home/you/.claude)
login:   home
dir:     /home/you/work/sessions/pglite_spike_20261017_024822
exec:    claude

$ agx new -p work "Fix: Login/Redirect!" --dry-run
profile: work (claude, /home/you/.claude-work)
login:   home
dir:     /home/you/work/sessions/fix_login_redirect_20261017_024822
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude

$ agx sessions ls
~/work/sessions (4 folders)
FOLDER                        LAST ACTIVITY  FILES  HISTORY
20261017_080000               just now       0      —
invoice_bug_20261017_024822   just now       0      —
pglite_spike_20261010_101500  just now       1      yes
20261001_090000               15 d ago       0      —

$ agx sessions gc
20261001_090000
1 empty folder(s) would be removed — re-run with --yes

$ agx sessions gc --min-age 0s
invoice_bug_20261017_024822
20261001_090000
2 empty folder(s) would be removed — re-run with --yes

$ agx sessions gc --yes
20261001_090000
removed 1 empty folder(s)

$ agx sessions promote pglite_spike_20261010_101500 pglite-go --dry-run
would move ~/work/sessions/pglite_spike_20261010_101500 → ~/work/pglite-go
  history in ~/.claude would move with it

$ agx sessions promote pglite_spike_20261010_101500 pglite-go --git-init
moved ~/work/sessions/pglite_spike_20261010_101500 → ~/work/pglite-go
  history in ~/.claude moved with it

$ cd ~/work/pglite-go && agx resume --last --dry-run
profile: personal (claude, /home/you/.claude)
login:   home
dir:     /home/you/work/pglite-go
exec:    claude --resume 9f1c2d3e-1111-4222-8333-944455566677
```

The folder list goes to stdout and the summary line to stderr, so `agx sessions gc | wc -l` counts folders. One record of `agx sessions ls --json`:

```json
{
  "name": "invoice_bug_20261017_024822",
  "path": "/home/you/work/sessions/invoice_bug_20261017_024822",
  "created": "2026-10-17T02:48:22Z",
  "modified": "2026-10-17T02:48:22.690893467Z",
  "files": 0,
  "has_history": false
}
```

## Pitfalls

- **`agx new` creates the folder before it launches.** The login is checked first, so `--login nope` stops with `agx: unknown login "nope"` and creates nothing. If the launch itself fails (for example `agx: claude not found on PATH`), the empty folder stays; `gc` removes it once it is older than `--min-age` (`internal/cli/launch.go`, `newNewCmd`). `agx new --dry-run` creates nothing.
- **Promote never crosses disks.** The move is a single rename, which the operating system cannot do between filesystems, so a destination on another disk stops with `agx: sessions: destination is on a different disk; promote only moves within one disk (move it yourself, e.g. with mv): <from> → <to>`, exit 1, and neither the folder nor its history moves (`sessions.ErrCrossDevice`; tested by `TestRenameError_CrossDevice` in `sessions/crossdevice_unix_test.go`).
- **Promote never overwrites.** An existing destination stops with `agx: sessions: destination already exists: <path>`, exit 1; a name such as `../x` stops with `agx: name must be a single folder name, got "../x"`, exit 2 (`internal/cli/sessions.go`, `validFolderName`).
- **Folders promoted outside agx lose their history link.** A plain `mv` leaves the Claude history keyed by the old path; resume in the new folder then finds nothing. Use `agx sessions promote`, or `agx sessions promote --to` for another parent on the same disk.
- **After a promote, the old path still shows in global listings.** Claude transcripts record the folder they started in and promote does not rewrite them, so `agx resume --all` lists the chat under the old folder, resuming it from there fails with `the conversation's folder … no longer exists`, and `agx ws` hides it. `cd` into the new folder and run `agx resume` there (`provider/claude/conversations.go`, `Conversations`; `TestPromoteThenResume` in `internal/cli/promote_resume_test.go` pins that resume from the new folder works).
- **`gc` only looks directly under `sessions.root`**, and a folder with any file in it is never removed, however old (`sessions/sessions.go`, `PlanGC`; tested by `TestListAndPlanGC`).
- **A bare name that does not exist is reported as a path in the current folder.** `agx sessions promote nosuch x` prints `agx: no such folder: <cwd>/nosuch`, exit 2, after `sessions.root` was also checked (`internal/cli/sessions.go`, `resolveFolder`).

## Related

- [Start an agent with `agx run`](run.md)
- [Resume a conversation](resume.md)
- [Move a folder's history to another account](move-history.md)
- [Shell integration](../config-shell/shell-integration.md) for landing in the new folder
- [JSON and scripting](../usage/json-and-scripting.md) for the `session.list`, `session.gc` and `session.promote` envelopes
- [README: Session folders](../../../README.md#session-folders-agx-sessions)
