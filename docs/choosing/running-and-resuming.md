# Running and resuming: which command

agx has four ways to start an agent (`run`, `run -p auto`, `new`, `resume` / `ws`) and three ways to tidy what you leave behind (`sessions gc`, `sessions promote`, `sessions move`). They differ in two things: which folder the agent starts in, and which account's home it runs on. This page lays the choices side by side so you can pick the right one without reading every guide first. Read it when you are about to start work, come back to it, or clean up after it.

## Before you start

- agx and at least one profile (`agx profiles`); see [Getting started](../guides/getting-started/README.md).
- For `new` and the `sessions` commands, `sessions.root` (default `~/agx-sessions`); see [Config reference: sessions](../guides/config-shell/config-reference.md#sessions).
- For landing in the new folder after `new`, `resume` and `ws`, the shell layer: `eval "$(agx shell-init zsh)"` ([Shell integration](../guides/config-shell/shell-integration.md)). Without it the agent still starts in the right folder, but your shell stays where it was.

## Step 1 — Pick the launch command

| You want to | Command | Folder | Home (account) | Guide | Example |
|---|---|---|---|---|---|
| Start in the folder you are in, on the default account | `agx run` | current | default profile of `--provider` (default `claude`) | [Run](../guides/running/run.md#step-1--run-on-the-default-profile) | [launch-command](../../examples/launch-command/) |
| Start here on a named account | `agx run -p work` | current | that profile's | [Pick a profile](../guides/running/run.md#step-2--pick-a-profile-with--p) | [launch-command](../../examples/launch-command/) |
| Start here on whichever plan has the most room | `agx run -p auto` | current | lowest highest-window percent; api-billed never | [Auto-pick](../guides/running/auto-pick.md) | [autopick](../../examples/autopick/) |
| Try something without cluttering a project | `agx new [slug words]` | a fresh `sessions.root/[slug_]YYYYMMDD_HHMMSS` | default, `-p NAME` or `-p auto` | [Session folders](../guides/running/new-and-sessions.md#step-1--start-in-a-new-folder) | [sessions-gc](../../examples/sessions-gc/) |
| Continue a chat started in this folder | `agx resume [words]` | the conversation's folder | the home that stores the conversation | [Resume](../guides/running/resume.md) | [conversations](../../examples/conversations/) |
| Continue a chat but you do not remember where | `agx ws` (or `agx resume -b`) | the chosen workspace | the home that stores the conversation | [Workspace picker](../guides/running/workspace-picker.md) | [conversations](../../examples/conversations/) |
| Continue the newest matching chat with no picker (scripts, ssh) | `agx resume --last [words]` (add `--all` for every folder) | the conversation's folder | the home that stores it | [Skip the picker](../guides/running/resume.md#step-3--skip-the-picker-with---last) | [conversations](../../examples/conversations/) |

If X then Y:

- If the agent needs extra flags every time, put them in the profile's `args:` ([Config reference: profiles](../guides/config-shell/config-reference.md#profiles)); they apply to every launch of that profile, `new` and `resume` included. For one run, put the flags after `--`: `run`, `new`, `resume` and `ws` all pass those words to the agent.
- If you want to spend another account's quota but keep this folder's chats, add `--login NAME` (or `--login auto`) to any of the commands above instead of switching profile; see [Which profile, which login](which-profile-or-login.md#step-4--choose--p-auto-or---login-auto).
- If you are unsure what a command will do, add `--dry-run` to `run`, `new`, `resume` or `ws`: it prints the profile, login, folder, env and command line with secrets redacted, and launches nothing.

## Step 2 — Know which account `resume` uses

`resume` and `ws` never ask you for a profile: a conversation lives in exactly one home, and agx launches it on a profile that uses that home. When several profiles share the home, it chooses a plan-billed profile before an api-billed one, then the provider's default, then config order (`config.ForHome`; [Resume: which profile](../guides/running/resume.md#step-6--know-which-profile-a-conversation-resumes-on)).

| You want | Use | Not |
|---|---|---|
| Same chat, same account | `agx resume` | — |
| Same chat, another account's quota | `agx resume --login NAME` | `resume -p OTHER`, which starts in the other home where the transcript does not exist |
| The chat itself to belong to another account from now on | `agx sessions move --to OTHER`, then `agx resume` | `resume -p` |
| Auto-pick | `agx resume --login auto` | `resume -p auto`, which is refused |

## Step 3 — Pick the cleanup command

| You want to | Command | What it touches | Safe by default | Guide | Example |
|---|---|---|---|---|---|
| See what is under `sessions.root` | `agx sessions ls` | nothing | — | [List the folders](../guides/running/new-and-sessions.md#step-2--list-the-folders) | [sessions-gc](../../examples/sessions-gc/) |
| Delete the `new` folders you never used | `agx sessions gc --yes` | only folders directly under `sessions.root` that are empty, have no conversation history and are older than `--min-age` (default 24h) | yes: without `--yes` it only lists | [Clean up with gc](../guides/running/new-and-sessions.md#step-3--clean-up-with-gc) | [sessions-gc](../../examples/sessions-gc/) |
| Keep a `new` folder as a real project | `agx sessions promote FOLDER NAME` | renames the folder to `<promote_root>/NAME` (default: the parent of `sessions.root`) and carries its Claude history along | refuses an existing destination and another disk | [Promote](../guides/running/new-and-sessions.md#step-4--promote-a-folder-into-a-project) | [sessions-gc](../../examples/sessions-gc/) |
| Move a folder's conversations to another Claude account | `agx sessions move --to PROFILE` | the conversation files and their folder history between two homes | backup first, rollback on failure, `--dry-run` shows the plan | [Move history](../guides/running/move-history.md) | [conversations](../../examples/conversations/) |

If X then Y:

- If you want a session folder somewhere else on the same disk, use `promote --to PARENT`; if it is on another disk, move it yourself with `mv`, knowing that Claude history keyed by the old path stays behind ([Session folders: pitfalls](../guides/running/new-and-sessions.md#pitfalls)).
- If the project is a Codex one, `sessions move` is refused on purpose (Codex indexes conversations by absolute path in SQLite); keep working on the account that holds them.
- If a folder has any file in it, `gc` keeps it however old it is; delete it by hand if you really mean to.

## Try it

A throwaway home whose `sessions.root` is `~/agx-sessions` (the home folder is shown as `/home/you`):

```text
$ agx new pglite spike --dry-run
profile: personal (claude, /home/you/.claude)
login:   home
dir:     /home/you/agx-sessions/pglite_spike_20261017_030858
exec:    claude
$ agx new pglite spike -p work -- --model opus
fake claude --model opus
$ agx sessions ls
~/agx-sessions (1 folders)
FOLDER                        LAST ACTIVITY  FILES  HISTORY
pglite_spike_20261017_035300  just now       0      —
$ agx sessions gc
nothing to remove
$ agx sessions gc --min-age 0s
pglite_spike_20261017_035300
1 empty folder(s) would be removed — re-run with --yes
```

In the second command the words before `--` name the folder and `--model opus` reaches the agent (a stand-in script that prints its arguments). `gc` keeps the folder until it is older than `--min-age`, and lists it without `--yes`.

## Pitfalls

- **Agent flags go after `--`.** Before it, `new` reads words as the folder name and `resume` / `ws` as a query, and a word starting with `-` is read as an agx flag (`agx: unknown flag: --model`, exit 2). `agx new pglite -- --model opus` names the folder `pglite_…` and starts the agent with `--model opus` (`internal/cli/launch.go`, `splitAtDash`).
- **`agx new` creates the folder before launching**, so a failed launch leaves an empty folder; `gc` removes it after `--min-age`. An unusable `--login` is refused before the folder is made ([Session folders: pitfalls](../guides/running/new-and-sessions.md#pitfalls)).
- **`-p auto` may land you in an account without this project's chats.** It chooses the home; `--login auto` chooses only the quota ([Auto-pick: pitfalls](../guides/running/auto-pick.md#pitfalls)).
- **`ws` hides folders that no longer exist**, including the old path of a promoted folder; `cd` into the new folder and run `agx resume` there ([Workspace picker: pitfalls](../guides/running/workspace-picker.md#pitfalls)).
- **No terminal, no picker.** Under cron, CI or `ssh host agx resume`, use `--last` or `--list` ([Resume: pitfalls](../guides/running/resume.md#pitfalls)).

## Related

- [Which profile, which login](which-profile-or-login.md)
- [Run](../guides/running/run.md), [Auto-pick](../guides/running/auto-pick.md), [Resume](../guides/running/resume.md), [Workspace picker](../guides/running/workspace-picker.md)
- [Session folders](../guides/running/new-and-sessions.md), [Move history](../guides/running/move-history.md)
