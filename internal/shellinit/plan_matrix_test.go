package shellinit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAgx stands in for the binary: `run` echoes its arguments one per
// bracket, `new` writes a plan, `resume` fails, `ws` writes an empty plan,
// and `exec --plan P [--print-dir]` prints $TARGET or where it launched.
const fakeAgx = `#!/bin/sh
case "$1" in
  run) for a in "$@"; do printf '[%s]' "$a"; done; echo; exit 0 ;;
  new) echo plan > "$AGX_PLAN_FILE"; exit 0 ;;
  resume) exit 3 ;;
  ws) exit 0 ;;
  exec) if [ "$4" = "--print-dir" ]; then printf '%s\n' "$TARGET"; else echo "launched in $(pwd)"; fi; exit 0 ;;
esac
exit 9
`

// TestScript_AliasesAndPlanProtocol runs the generated script end to end in
// each installed shell: alias arguments arrive as separate words (user args
// with spaces and $ intact), `new` cds into the plan's directory before the
// final exec, a failing or plan-less command returns its exit code without
// moving and removes its plan file.
func TestScript_AliasesAndPlanProtocol(t *testing.T) {
	t.Parallel()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "agx"), []byte(fakeAgx), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "wörk dir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	const body = `cl "a b" 'c$d' ''
start=$PWD
agx resume; echo "resume rc=$? moved=$([ "$PWD" = "$start" ] && echo no || echo yes)"
agx ws; echo "ws rc=$? moved=$([ "$PWD" = "$start" ] && echo no || echo yes)"
clnew x; echo "new rc=$? pwd=$PWD"
`
	ran := 0
	for _, sh := range Shells {
		shBin, err := exec.LookPath(string(sh))
		if err != nil {
			t.Logf("%s not installed", sh)
			continue
		}
		ran++
		tmp := t.TempDir()
		path := filepath.Join(t.TempDir(), "init."+string(sh))
		script := render(t, sh, Alias{Name: "cl", Args: "run -p personal"}, Alias{Name: "clnew", Args: "new -p work"}) + "\n" + body
		if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(shBin, path)
		cmd.Dir = t.TempDir()
		cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + t.TempDir(), "TMPDIR=" + tmp, "TARGET=" + target, "HOST=test"}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", sh, err, out)
		}
		got := string(out)
		for _, want := range []string{
			"[run][-p][personal][a b][c$d][]\n",
			"resume rc=3 moved=no\n",
			"ws rc=0 moved=no\n",
			"launched in " + target + "\n",
			"new rc=0 pwd=" + target + "\n",
			"\033]7;file://test" + target,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: output lacks %q:\n%s", sh, want, got)
			}
		}
		// Only new's plan remains (the real exec consumes it); the failed
		// and empty ones were removed by the function.
		left, err := os.ReadDir(tmp)
		if err != nil {
			t.Fatal(err)
		}
		if len(left) != 1 {
			t.Errorf("%s: plan files left in TMPDIR: %v", sh, left)
		}
	}
	if ran == 0 {
		t.Fatal("no shell installed")
	}
}
