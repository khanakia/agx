// Package examples holds runnable programs that use agx's Go library, one
// per folder. This test builds and runs every one of them and checks its
// output, so an example that stops compiling or starts printing something
// else fails `go test ./...` (and therefore `task check`) instead of rotting
// in the docs that quote it.
package examples

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// goBinary is the toolchain used to build the examples; the test fails
// rather than skips without it, because a skipped example is an unverified
// one.
const goBinary = "go"

// want lists, per example folder, lines its output must contain. Each line
// pins a behaviour the matching guide in docs/guides/library/ describes.
var want = map[string][]string{
	"usage-fake-server": {
		`home login: logged in=true plan="Max (20x)"`,
		"weekly  All models        76.0% warning  resets 2026-09-24T17:59:59Z (binding)",
		"weekly  Fable             68.0% normal",
		"  max: 76%",
		"weekly  All models        81.0% warning  resets 2026-09-29T17:59:59Z (binding)",
		"(retry after 30s) (rate-limited=true)",
		"login rejected by the server (HTTP 401) (unauthorized=true)",
	},
	"autopick": {
		"auto → work (max 12%) over personal (max 77%)",
		"skipped kimi: billed per token: no usage windows",
		"skipped spare: not logged in",
		"auto → a (max 40%) over b (max 40%)",
		"(no-candidate=true)",
	},
	"config-resolve": {
		"no file (found=false), sessions root ~/agx-sessions",
		"work     claude ~/.claude-work  plan default=false discovered",
		"with file (found=true), sessions root ~/work/sessions, promote root ~/work",
		"kimi     claude ~/.claude-work  api  default=false config",
		"codex    codex  ~/.codex        plan default=true  discovered",
		"resume in ~/.claude-work runs on: main (plan before api)",
		"alias clm → agx run -p main",
		"field profile not found in type config.File (invalid=true)",
		"shell.aliases.bad: only letters, digits, spaces and _./:=@+- are allowed",
	},
	"conversations": {
		"history key for /srv/demo/api: -srv-demo-api",
		`a1  /srv/demo/api     "Login fix (final)"`,
		"has history: /srv/demo/api=true /srv/demo/none=false",
		`a1  /srv/demo/api-v2  "Login fix (final)"`,
		"plan: 2 conversation(s), 2 item(s), 0 blocker(s)",
		"after apply, ~/.claude-work holds 1 conversation(s) for /srv/demo/web",
	},
	"launch-command": {
		"personal (default home):\n  argv: claude\n  dir:  /srv/demo/api\n  env:  ANTHROPIC_API_KEY=<set>\n",
		"argv: claude --model opus --resume a1 --verbose",
		"env:  CLAUDE_CODE_OAUTH_TOKEN=<set> CLAUDE_CONFIG_DIR=/home/demo/.claude-work",
		"argv: codex resume 019a-demo",
		"CODEX_HOME=/home/demo/.codex-team",
	},
	"sessions-gc": {
		"same-second twin: chat_20261003_090000_2",
		"newer than min age",
		"has conversation history",
		"notes_20261002_090000            files=1 remove=false has files",
		"removed try_the_new_api_20261001_090000",
		"remove non-empty: refused=true",
		"promote onto existing: exists=true",
	},
	"login-resolve": {
		"token file written with mode 0600",
		"personal source=stored resolved 22 chars, owned by agx=true",
		"team     source=cmd    resolved 19 chars, owned by agx=false",
		"personal expires 2027-10-01",
		"missing stored: true",
		"token has more than one line",
		"profile kimi sets ANTHROPIC_API_KEY in env",
		"world-readable token file: insecure=true",
	},
	"secret-refs": {
		`parse "env:KIMI_TOKEN"     scheme=env path=KIMI_TOKEN`,
		`error: secret: reference must be scheme:ref: "KIMI_TOKEN"`,
		"system: ANTHROPIC_AUTH_TOKEN resolved (15 chars)",
		"custom: ANTHROPIC_API_KEY resolved (16 chars)",
		"secret: env MISSING_TOKEN is not set",
		"(empty=true)",
	},
	"custom-provider": {
		"spare  default=false max=  0% err=not logged in",
		"auto → work (max 35%) over echo (max 90%)",
		"env:  PATH=/usr/bin ECHOAGENT_HOME=~/.echoagent-work",
	},
}

// secrets are the fake token values the examples use; none may ever be
// printed, which is the behaviour the examples teach.
var secrets = []string{
	"fake-home-access-token", "fake-setup-token", "fake-api-key",
	"fake-stored-token-0001", "fake-env-token-0002", "fake-cmd-token-0003",
	"fake-kimi-token", "fake-vault-token",
}

func TestExamples(t *testing.T) {
	t.Parallel()
	goBin, err := exec.LookPath(goBinary)
	if err != nil {
		t.Fatalf("the examples need the go toolchain on PATH: %v", err)
	}
	dirs, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		seen++
		name := d.Name()
		lines, ok := want[name]
		if !ok {
			t.Errorf("examples/%s has no expected output in want", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bin := filepath.Join(t.TempDir(), name)
			build := exec.Command(goBin, "build", "-o", bin, ".")
			build.Dir = name
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build: %v\n%s", err, out)
			}
			out, err := exec.Command(bin).CombinedOutput()
			if err != nil {
				t.Fatalf("run: %v\n%s", err, out)
			}
			got := string(out)
			for _, l := range lines {
				if !strings.Contains(got, l) {
					t.Errorf("output lacks %q\n--- output ---\n%s", l, got)
				}
			}
			for _, s := range secrets {
				if strings.Contains(got, s) {
					t.Errorf("output leaks the fake secret %q", s)
				}
			}
		})
	}
	if seen != len(want) {
		t.Errorf("found %d example folders, want lists %d", seen, len(want))
	}
}
