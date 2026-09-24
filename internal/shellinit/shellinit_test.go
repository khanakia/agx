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
		"new|resume)",
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
