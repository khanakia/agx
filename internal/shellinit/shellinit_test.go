package shellinit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func render(t *testing.T, sh Shell, aliases ...Alias) string {
	t.Helper()
	var b strings.Builder
	if err := Write(&b, Params{Shell: sh, Aliases: aliases}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestWrite(t *testing.T) {
	t.Parallel()
	out := render(t, Zsh, Alias{Name: "cl", Args: "run -p personal"}, Alias{Name: "clw", Args: "new -p work"})
	for _, want := range []string{
		"export AGX_SHELL_INIT=zsh",
		"new|resume|ws)",
		`AGX_PLAN_FILE="$plan" command agx "$@"`,
		`command agx exec --plan "$plan" --print-dir`,
		"cl() { agx run -p personal \"$@\"; }",
		"clw() { agx new -p work \"$@\"; }",
		"command rm -f --", // never the user's aliased rm
	} {
		if !strings.Contains(out, want) {
			t.Errorf("script missing %q\n%s", want, out)
		}
	}
}

func TestWrite_Unsupported(t *testing.T) {
	t.Parallel()
	if err := Write(&strings.Builder{}, Params{Shell: "fish"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v", err)
	}
}

// TestScriptParses runs each supported shell's syntax checker over the
// generated script, so a template edit that breaks the shell fails here
// instead of in the user's rc file. Shells not installed are skipped.
func TestScriptParses(t *testing.T) {
	t.Parallel()
	for _, sh := range Shells {
		bin, err := exec.LookPath(string(sh))
		if err != nil {
			t.Logf("%s not installed; skipping syntax check", sh)
			continue
		}
		path := filepath.Join(t.TempDir(), "init."+string(sh))
		if err := os.WriteFile(path, []byte(render(t, sh, Alias{Name: "cl", Args: "run -p personal"})), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(bin, "-n", path).CombinedOutput(); err != nil {
			t.Errorf("%s -n failed: %v\n%s", sh, err, out)
		}
	}
}

// TestAglogin runs the generated aglogin function in each installed shell
// against a fake agx whose `login exists` knows "work" and "home": a known
// name is exported, no argument unsets, and an unknown name fails and
// leaves the previous value alone. bash must be installed (every supported
// platform has it); other shells are checked when present.
func TestAglogin(t *testing.T) {
	t.Parallel()
	bin := t.TempDir()
	fake := "#!/bin/sh\n[ \"$1 $2\" = \"login exists\" ] || exit 9\ncase \"$3\" in work|home) exit 0 ;; esac\necho \"agx: unknown login \\\"$3\\\"\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "agx"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	const script = `aglogin work; echo "1 rc=$? v=${AGX_LOGIN-unset}"
aglogin nosuch; echo "2 rc=$? v=${AGX_LOGIN-unset}"
aglogin home; echo "3 rc=$? v=${AGX_LOGIN-unset}"
aglogin; echo "4 rc=$? v=${AGX_LOGIN-unset}"
aglogin nosuch; echo "5 rc=$? v=${AGX_LOGIN-unset}"
sh -c 'echo "6 child=${AGX_LOGIN-unset}"'
aglogin work; sh -c 'echo "7 child=${AGX_LOGIN-unset}"'
`
	const want = "1 rc=0 v=work\n2 rc=1 v=work\n3 rc=0 v=home\n4 rc=0 v=unset\n5 rc=1 v=unset\n6 child=unset\n7 child=work\n"
	ran := 0
	for _, sh := range Shells {
		shBin, err := exec.LookPath(string(sh))
		if err != nil {
			if sh == Bash {
				t.Fatalf("bash not installed: %v", err)
			}
			t.Logf("%s not installed; aglogin checked in the other shells", sh)
			continue
		}
		ran++
		path := filepath.Join(t.TempDir(), "init."+string(sh))
		if err := os.WriteFile(path, []byte(render(t, sh)+"\n"+script), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(shBin, path)
		cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + t.TempDir()}
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", sh, err, stderr.String())
		}
		if stdout.String() != want {
			t.Errorf("%s: got\n%s\nwant\n%s\nstderr:\n%s", sh, stdout.String(), want, stderr.String())
		}
		for _, msg := range []string{`unknown login "nosuch"`, "AGX_LOGIN=work in this terminal", "AGX_LOGIN unset"} {
			if !strings.Contains(stderr.String(), msg) {
				t.Errorf("%s: stderr lacks %q:\n%s", sh, msg, stderr.String())
			}
		}
	}
	if ran == 0 {
		t.Fatal("no shell ran aglogin")
	}
}
