package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBinary builds the real agx binary and runs it against an empty,
// isolated HOME, pinning main's wiring: exit codes and that errors reach
// stderr exactly once.
func TestBinary(t *testing.T) {
	t.Parallel()
	bin := filepath.Join(t.TempDir(), "agx")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	run := func(args ...string) (int, string, string) {
		cmd := exec.Command(bin, args...)
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "AGX_CONFIG_DIR=" + filepath.Join(home, "cfg")}
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		var exitErr *exec.ExitError
		switch {
		case err == nil:
			return 0, stdout.String(), stderr.String()
		case errors.As(err, &exitErr):
			return exitErr.ExitCode(), stdout.String(), stderr.String()
		default:
			t.Fatalf("run %v: %v", args, err)
			return -1, "", ""
		}
	}

	if code, out, _ := run("--version"); code != 0 || !strings.HasPrefix(out, "agx version") {
		t.Errorf("--version: %d %q", code, out)
	}
	if code, _, errOut := run("no-such-command"); code != 2 || strings.Count(errOut, "agx:") != 1 {
		t.Errorf("unknown command: exit %d, stderr %q", code, errOut)
	}
	// Empty HOME: nothing to show is a usage error with a hint, not a crash.
	if code, _, errOut := run("usage"); code != 2 || !strings.Contains(errOut, "agx doctor") {
		t.Errorf("usage on empty home: exit %d, stderr %q", code, errOut)
	}
	if code, out, _ := run("profiles", "--json"); code != 0 || !strings.Contains(out, `"kind": "profile.list"`) {
		t.Errorf("profiles --json: %d %q", code, out)
	}
	if code, out, _ := run("shell-init", "zsh"); code != 0 || !strings.Contains(out, "agx()") {
		t.Errorf("shell-init: %d", code)
	}
}
