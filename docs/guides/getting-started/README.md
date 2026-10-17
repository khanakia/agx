# Install agx and run it the first time

agx is one command for Claude Code and Codex across every account on your machine: it shows plan usage for all of them, launches an agent on the account with the most headroom, and resumes a conversation on the account that holds it. This guide installs the binary, checks that the download is genuine, and walks through the first three commands to run on a new machine. Read it once when you set agx up; after that, [Find and add accounts](accounts.md) covers adding a second account.

## Before you start

- macOS or Linux on amd64 or arm64, or Windows on amd64. Releases ship `darwin_amd64`, `darwin_arm64`, `linux_amd64`, `linux_arm64` (`.tar.gz`) and `windows_amd64` (`.zip`), plus one `checksums.txt`.
- `curl` and `sha256sum` (Linux) or `shasum` (macOS) for the download route, or Go 1.26.4 or newer for `go install` (the version `go.mod` asks for).
- Claude Code and/or Codex already installed and logged in at least once, if you want agx to find accounts. agx works without them, but there is then nothing to show.

## Step 1 — Download a release and check its checksum

The release page publishes a SHA-256 for every archive in `checksums.txt`. Checking it proves the archive you got is the one the release workflow built. Pick your platform, then run the whole block:

```sh
VERSION=v0.3.3
PLATFORM=linux_amd64        # or darwin_arm64, darwin_amd64, linux_arm64
curl -sLO "https://github.com/khanakia/agx/releases/download/${VERSION}/agx_${VERSION}_${PLATFORM}.tar.gz"
curl -sLO "https://github.com/khanakia/agx/releases/download/${VERSION}/checksums.txt"
grep "agx_${VERSION}_${PLATFORM}.tar.gz" checksums.txt | sha256sum -c     # macOS: shasum -a 256 -c
tar -xzf "agx_${VERSION}_${PLATFORM}.tar.gz" agx
sudo mv agx /usr/local/bin/
```

The `grep … | sha256sum -c` line must print `OK`. If it prints `FAILED`, delete the archive and download it again; do not install it.

On Windows, download `agx_v0.3.3_windows_amd64.zip` and its line in `checksums.txt` from the [Releases page](https://github.com/khanakia/agx/releases), check it with `certutil -hashfile agx_v0.3.3_windows_amd64.zip SHA256`, and put `agx.exe` on your `PATH`.

## Step 2 — Or install with Go

If you already have Go, this builds the latest tagged release from source and puts `agx` in `$(go env GOPATH)/bin`, which must be on your `PATH`:

```sh
go install github.com/khanakia/agx@latest
```

To build from a clone instead (needs [Task](https://taskfile.dev); `task install` runs `go install` from the clone):

```sh
git clone https://github.com/khanakia/agx.git
cd agx
task install
```

## Step 3 — Check the version

```sh
agx version
```

`agx version` prints the version, the commit it was built from, the build time, the Go version and the platform. `agx --version` prints one line, and `agx version --json` prints the same facts as a [`version.show`](../usage/json-and-scripting.md#versionshow) envelope.

## Step 4 — Run the three first-run commands

No config file is needed. agx looks for Claude Code homes (`~/.claude`, `~/.claude-<name>`) and the Codex home (`~/.codex`) and turns each one into a profile; [Find and add accounts](accounts.md) explains the rules.

```sh
agx profiles     # which accounts agx found, and whether each is logged in
agx doctor       # local health check: config, profiles, logins, binaries, shell layer
agx              # plan usage for every account (same as `agx usage`)
```

`agx profiles` and `agx doctor` read local files only. `agx` makes one read-only request per logged-in, plan-billed account to the vendor's usage endpoint; it never calls a model and never refreshes a login.

## Step 5 — Load the shell layer (optional)

`agx new`, `agx resume` and `agx ws` can leave your shell in the session folder they started in, but only when the shell layer is loaded. Add one line to your shell rc file:

```sh
echo 'eval "$(agx shell-init zsh)"' >> ~/.zshrc      # bash: agx shell-init bash, in ~/.bashrc
```

Open a new terminal afterwards; `agx doctor` then shows `✓ shell  shell layer active (zsh)`. See [Shell integration](../config-shell/shell-integration.md) for what the layer defines.

## Try it

Checking and installing the Linux amd64 archive of v0.3.3:

```text
$ grep "agx_${VERSION}_${PLATFORM}.tar.gz" checksums.txt | sha256sum -c
agx_v0.3.3_linux_amd64.tar.gz: OK
$ agx version
agx v0.3.3
  source:      ldflags
  commit:      6314c2a406c34c2252a595a775d6820748cd8ced
  built:       2026-10-17T02:35:10Z
  go:          go1.26.4
  platform:    linux/amd64
$ agx --version
agx version v0.3.3
```

The same version built with `go install` reports where its version came from differently, because `go install` does not stamp a commit or build time:

```text
$ agx version
agx v0.3.3
  source:      module
  commit:      unknown
  built:       unknown
  go:          go1.26.5
  platform:    linux/amd64
```

The first run on a machine with no Claude Code or Codex home yet:

```text
$ agx profiles
PROFILE  PROVIDER  HOME  ACCOUNT  STATE  LOGIN  BILLING  SOURCE
$ agx doctor
· config             no file at ~/.agx/config.yaml — using discovered profiles and defaults
· sessions.root      ~/agx-sessions does not exist yet (created by `agx new`)
· shell              shell layer not loaded — add `eval "$(agx shell-init zsh)"` to your shell rc so new/resume can cd
$ agx
agx: no profiles to show (run `agx doctor`)
$ echo $?
2
```

## Pitfalls

- **No accounts is a usage error, not an empty list.** `agx` / `agx usage` exit 2 with ``no profiles to show (run `agx doctor`)`` when no home was found (`internal/cli/usage.go`, `runUsage`), and `agx plan` says ``no plan-billed profiles (run `agx doctor`)`` (`internal/cli/plan.go`). Log in with Claude Code or Codex once, then run `agx profiles` again.
- **`go install` builds lack a commit and build time.** `agx version` shows `source: module`, `commit: unknown`. Use a release archive when you need the provenance fields filled in (seen above; the values come from voltkit's `versioncmd`).
- **There is no Windows arm64 build.** The v0.3.3 release has `windows_amd64.zip` only. On Windows, agx cannot replace its own process, so `run` starts the agent as a child process and waits (`internal/cli/exec_other.go`); Windows is otherwise untested.
- **The macOS Keychain is read only on macOS.** On Linux, a Claude Code login is read from `<home>/.credentials.json`; the Keychain lookup needs the macOS `security` tool and is skipped when it is missing (`provider/claude/keychain.go`).
- **The shell layer is opt-in.** Without it, `agx new` and `agx resume` still start the agent in the right folder, but your shell does not change directory afterwards, and `agx doctor` reports `· shell  shell layer not loaded` (`internal/cli/doctor.go`).

## Related

- [Find and add accounts](accounts.md): how discovery names profiles, and adding a second Claude Code or Codex account.
- [Usage, plans and profiles](../usage/usage-and-plans.md): reading `agx`, `agx plan` and `agx profiles`.
- [Health check: agx doctor](../usage/doctor.md): every check and how to fix it.
- [Troubleshooting](../usage/troubleshooting.md): error messages and what to do about them.
- [Auto-pick](../running/auto-pick.md): `agx run -p auto`.
- [Config reference](../config-shell/config-reference.md): the optional `~/.agx/config.yaml`.
