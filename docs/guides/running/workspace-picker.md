# Workspace picker: `agx ws`

`agx ws` is for the moment you know you had a conversation but not which folder it was in. It lists every folder that has conversations, across every account and provider, newest activity first; you pick a folder, then a chat in it, and it resumes in that folder on the account that stores it. `agx ws` is the short name for `agx resume --browse` (`agx resume -b`); both run the same code.

## Before you start

- The examples use the config from [Start an agent with `agx run`](run.md#before-you-start) and three folders with conversations: `~/code/app`, `~/code/docker_setup_mac` and one session folder.
- Read [Resume a conversation](resume.md) first: the chat list, the profile column and the profile choice are the same as in `agx resume`.

## Step 1 — Browse workspaces, then chats

```sh
agx ws
```

The first list shows one row per folder: last activity, how many chats it holds, the profiles those chats resume on (newest first), and the folder. Pick one and the second list shows that folder's chats in the `resume` row format. Pick a chat and it resumes.

Esc on the chat list (or Enter on an empty numbered prompt) goes back to the folder list; Esc on the folder list exits with code 1. You can run `agx ws` from any folder: it always searches every folder.

## Step 2 — Narrow the folders with words

```sh
agx ws invoice
```

Only chats whose title, folder or id contain every word are kept, and only folders with at least one such chat are listed; the chat list then holds only the matching chats.

## Step 3 — Print the folders

```sh
agx ws --list
agx ws --list --json
```

`--list` prints the folder rows; `--json` prints a `workspace.list` envelope with `dir`, `updated`, `count` and `profiles` for each folder.

## Step 4 — Combine with the launch flags

`ws` takes `--dry-run`, `-p` (resume on another profile, as in `resume`) and `--login` (spend another login's quota, see [Logins](../logins/overview.md)):

```sh
agx ws --dry-run
agx ws invoice --dry-run
```

## Try it

```text
$ agx ws --list
3 min ago    4 chats   personal,work,codex  ~/code/app
10 hr ago    1 chat    personal             ~/work/sessions/pglite_spike_20261010_101500
1 d ago      3 chats   personal             ~/code/docker_setup_mac

$ agx ws invoice --list
40 min ago   1 chat    work                 ~/code/app
```

```text
$ agx ws --list --json
{
  "schema_version": 1,
  "kind": "workspace.list",
  "count": 3,
  "data": [
    {
      "dir": "/home/you/code/app",
      "updated": "2026-10-17T02:45:47.929212898Z",
      "count": 4,
      "profiles": [
        "personal",
        "work",
        "codex"
      ]
    },
    {
      "dir": "/home/you/work/sessions/pglite_spike_20261010_101500",
      "updated": "2026-10-16T16:48:48.016220706Z",
      "count": 1,
      "profiles": [
        "personal"
      ]
    },
    {
      "dir": "/home/you/code/docker_setup_mac",
      "updated": "2026-10-16T02:48:47.979503975Z",
      "count": 3,
      "profiles": [
        "personal"
      ]
    }
  ]
}
```

Without fzf both levels are numbered lists; here folder 1, then chat 1:

```text
$ agx ws --dry-run
  1) 3 min ago    4 chats   personal,work,codex  ~/code/app
  2) 10 hr ago    1 chat    personal             ~/work/sessions/pglite_spike_20261010_101500
  3) 1 d ago      3 chats   personal             ~/code/docker_setup_mac
workspace [1-3, Enter to cancel]: 1
  1) 3 min ago    claude personal   Fix the login redirect loop      ~/code/app                    2bec3f80
  2) 40 min ago   claude work       Invoice export to CSV            ~/code/app                    dbcaeaf2
  3) 1 hr ago     codex  codex      (untitled)                       ~/code/app                    019eea1d
  4) 1 hr ago     claude personal   Add pagination to orders         ~/code/app                    5c9565db
chat (esc: back) [1-4, Enter to cancel]: 1
profile: personal (claude, /home/you/.claude)
login:   home
dir:     /home/you/code/app
exec:    claude --resume 2bec3f80-6a1e-4c55-9d0e-3f1a7c2b9e11
```

## Pitfalls

- **Folders that no longer exist are hidden.** A deleted or unmounted folder can be neither resumed in nor moved, so it is left out rather than offered and then refused (`internal/cli/browse.go`, `loadWorkspaces`).
- **A promoted folder's Claude chats disappear from `ws`.** `ws` groups chats by the folder recorded in each transcript, and `agx sessions promote` does not rewrite transcripts, so those chats point at the old, now missing folder and are hidden. `cd` into the new folder and run `agx resume` (`provider/claude/conversations.go`, `Conversations`).
- **`ws` has no `--last`, `--all` or `--limit`.** It always searches every folder without a cap; `agx ws --last` fails with `agx: unknown flag: --last`, exit 2. Use `agx resume --all --last <words>` to resume without a picker (`internal/cli/browse.go`, `newWsCmd`).
- **Every transcript is opened.** Listing every folder means reading the head of every transcript on the machine (only the first and last 256 KiB of each, never the whole file), so on a home with years of history `ws` takes longer than a `resume` in one folder, which reads only that folder's history.
- **No terminal, no picker.** As in `resume`, `ws` without a terminal fails with `agx: no terminal to pick from (use --last, or --list to see ids)`; use `agx ws --list` to print instead.
- **The profiles column is cut at 20 characters.** `--json` carries the full list (`internal/cli/browse.go`, `profilesWidth`).

## Related

- [Resume a conversation](resume.md)
- [Move a folder's history to another account](move-history.md): `agx sessions move -i --all` uses the same folder picker
- [JSON and scripting](../usage/json-and-scripting.md) for the `workspace.list` envelope
- [README: Pick up where you left off](../../../README.md#pick-up-where-you-left-off-agx-resume)
