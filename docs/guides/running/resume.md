# Resume a conversation on the account that holds it

`agx resume` lists the conversations started in the current folder across every Claude and Codex home on the machine, lets you pick one, and continues it in its own folder on the profile whose home stores it. A chat you had on `work` resumes on `work`, a Codex chat resumes with `codex resume`, and you never have to remember which account you used. Use it instead of `claude --resume` whenever you have more than one account.

## Before you start

- The examples use the config from [Start an agent with `agx run`](run.md#before-you-start), and a project folder `~/code/app` with four conversations: two on `personal`, one on `work` and one on Codex.
- [fzf](https://github.com/junegunn/fzf) gives you a filterable picker. Without it agx falls back to a numbered list read from the terminal, so fzf is optional.

## Step 1 — Pick from this folder's conversations

```sh
cd ~/code/app
agx resume
```

The picker lists one conversation per row, newest first: how long ago it was last active, the provider, the profile it resumes on, the title (`(untitled)` when the agent never named it), the folder, and the first 8 characters of the id. Pick one and it resumes in its folder.

In a folder with no conversations, `resume` stops with `no conversations in <folder>` and a hint to use `--all`, exit 1. It never widens the search on its own.

## Step 2 — Filter with words

```sh
agx resume invoice
agx resume login redirect
agx resume dbcaeaf2
```

Every word must appear, case-insensitively, in the title, the folder path or the id. When exactly one conversation matches, it resumes without a picker. A query searches all conversations before `--limit` is applied, so an old match is not cut off.

## Step 3 — Skip the picker with `--last`

```sh
agx resume --last
agx resume invoice --last
```

`--last` takes the newest match. This is also the way to resume from a script or any place without a terminal.

## Step 4 — Search every folder with `--all`

```sh
agx resume --all
agx resume --all --last invoice
agx resume --all --list --limit 20
```

`--all` lists conversations from every folder. `--limit` (default 50) caps how many are considered, newest first.

## Step 5 — Print instead of launching with `--list`

```sh
agx resume --list
agx resume --all --list --json
```

`--list` prints the same rows the picker shows. `--list --json` prints a `conversation.list` envelope with the full id, folder and home of each conversation, plus `profile` (the profile it resumes on) and `profiles` (every profile that uses that home). An empty folder prints an empty `data` list with exit 0, so scripts always get valid JSON.

## Step 6 — Know which profile a conversation resumes on

The conversation's home decides it. A conversation stored in `~/.claude-work` resumes on the profile whose home is `~/.claude-work`. When several profiles share one home, such as `personal` (plan-billed) and `kimi` (api-billed) on `~/.claude`, the transcript does not record which one made it, so agx chooses in this order:

1. a plan-billed profile before an api-billed one,
2. then the provider's default profile,
3. then config order.

The list, the picker and the launch all use this same rule (`config.ForHome`), so the profile column always names the profile the launch will use. `--json` adds `profiles` with every candidate.

## Step 7 — Override the profile with `-p` (advanced)

```sh
agx resume -p work 2bec3f80 --dry-run
```

`-p` launches the chosen conversation on another profile. The other profile only finds the conversation when it uses the same home; to really switch a project's conversations to another account, use [`agx sessions move`](move-history.md), and to spend another account's quota on the same home, use `--login` ([Logins](../logins/overview.md)).

## Try it

```text
$ cd ~/code/app && agx resume --list
3 min ago    claude personal   Fix the login redirect loop      ~/code/app                    2bec3f80
40 min ago   claude work       Invoice export to CSV            ~/code/app                    dbcaeaf2
1 hr ago     codex  codex      (untitled)                       ~/code/app                    019eea1d
1 hr ago     claude personal   Add pagination to orders         ~/code/app                    5c9565db

$ agx resume invoice --list
40 min ago   claude work       Invoice export to CSV            ~/code/app                    dbcaeaf2

$ agx resume dbcaeaf2 --last --dry-run
profile: work (claude, /home/you/.claude-work)
login:   home
dir:     /home/you/code/app
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude --resume dbcaeaf2-7d1c-4e8b-b3a4-2c5f9e8d7a33

$ agx resume 019eea1d --last --dry-run
profile: codex (codex, /home/you/.codex)
login:   home
dir:     /home/you/code/app
exec:    codex resume 019eea1d-4c2b-7f00-9a1b-2d3e4f5a6b7c

$ agx resume --all --list
3 min ago    claude personal   Fix the login redirect loop      ~/code/app                    2bec3f80
40 min ago   claude work       Invoice export to CSV            ~/code/app                    dbcaeaf2
1 hr ago     codex  codex      (untitled)                       ~/code/app                    019eea1d
1 hr ago     claude personal   Add pagination to orders         ~/code/app                    5c9565db
10 hr ago    claude personal   Explore PGlite from Go           …/pglite_spike_20261010_101…  9f1c2d3e
1 d ago      claude personal   Hatchet, ClickHouse, HyperDX se… ~/code/docker_setup_mac       aeba9843
1 d ago      claude personal   (untitled)                       ~/code/docker_setup_mac       33f18428
1 d ago      claude personal   (untitled)                       ~/code/docker_setup_mac       6144697a

$ cd ~/code && agx resume
agx: no conversations in /home/you/code
     (agx resume --all to pick from every folder)
```

The first record of `agx resume --list --json` in `~/code/app` shows the shared home: the conversation resumes on `personal`, and `kimi` could have made it too.

```json
{
  "provider": "claude",
  "home": "/home/you/.claude",
  "id": "2bec3f80-6a1e-4c55-9d0e-3f1a7c2b9e11",
  "title": "Fix the login redirect loop",
  "dir": "/home/you/code/app",
  "updated": "2026-10-17T02:45:47.929212898Z",
  "profile": "personal",
  "profiles": [
    "personal",
    "kimi"
  ]
}
```

Without fzf, the picker is a numbered list on the terminal. Type a number and Enter; Enter alone cancels:

```text
$ agx resume --dry-run
  1) 3 min ago    claude personal   Fix the login redirect loop      ~/code/app                    2bec3f80
  2) 40 min ago   claude work       Invoice export to CSV            ~/code/app                    dbcaeaf2
  3) 1 hr ago     codex  codex      (untitled)                       ~/code/app                    019eea1d
  4) 1 hr ago     claude personal   Add pagination to orders         ~/code/app                    5c9565db
resume [1-4, Enter to cancel]: 2
profile: work (claude, /home/you/.claude-work)
login:   home
dir:     /home/you/code/app
env:     CLAUDE_CONFIG_DIR=/home/you/.claude-work
exec:    claude --resume dbcaeaf2-7d1c-4e8b-b3a4-2c5f9e8d7a33
```

## Pitfalls

- **No terminal, no picker.** The picker draws on `/dev/tty`, not stdin, so it works under redirection; but with no terminal at all (cron, CI, `ssh host agx resume`) it stops with `agx: no terminal to pick from (use --last, or --list to see ids)`, exit 1. Use `--last` with a query (`internal/cli/picker.go`, `ErrNoTerminal`).
- **Cancelling is exit 1.** Esc in fzf, Enter on an empty numbered prompt, or a query fzf cannot match all print `agx: cancelled` and exit 1; a number out of range prints `agx: not a choice between 1 and 4: "9"` (`internal/cli/picker.go`; tested by `TestPickNumbered` in `internal/cli/picker_test.go`).
- **A conversation whose folder is gone cannot resume.** agx refuses with ``the conversation's folder … no longer exists — if you moved it (e.g. `agx sessions promote`), cd into the new folder and run `agx resume` there``, because the agent must start in that folder (`internal/cli/launch.go`, `resumeConversation`).
- **`--all` shows the folder recorded in the transcript.** After `agx sessions promote`, a Claude chat still lists under its old folder in `--all`; inside the new folder, plain `agx resume` finds it under the right path (`provider/claude/conversations.go`, `Conversations`).
- **`-p` does not move a conversation.** `agx resume -p work` on a conversation stored in `~/.claude` starts Claude Code with `CLAUDE_CONFIG_DIR=~/.claude-work`, where that transcript does not exist. Move it first with [`agx sessions move`](move-history.md).
- **`-p` with a name that is not a profile** (including `auto`) fails with `no profile uses <home> (claude); add one to the config` instead of naming the unknown profile (`internal/cli/launch.go`, `resumeConversation`). Check the spelling against `agx profiles`.
- **A home no profile uses** shows its folder name in the profile column, and resuming one of its conversations fails with `no profile uses <home> (<provider>); add one to the config`. Add a profile for that home.
- **Ambiguous short ids are fine as queries.** A query matches by substring, so `agx resume 2bec` resumes directly when one conversation matches and opens the picker when several do.

## Related

- [Workspace picker: `agx ws`](workspace-picker.md) when you do not remember the folder
- [Move a folder's history to another account](move-history.md)
- [Session folders](new-and-sessions.md), including what promote does to history
- [JSON and scripting](../usage/json-and-scripting.md) for the `conversation.list` envelope
- [Shell integration](../config-shell/shell-integration.md): with the shell layer, your shell is left in the conversation's folder
- [README: Pick up where you left off](../../../README.md#pick-up-where-you-left-off-agx-resume)
