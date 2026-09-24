//go:build unix

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// execHelperEnv makes the test binary act as the process that calls
// execReplace, so the test observes what actually replaced it.
const execHelperEnv = "AGX_TEST_EXEC_HELPER"

func TestExecReplace(t *testing.T) {
	if os.Getenv(execHelperEnv) == "1" {
		err := execReplace(provider.Command{
			Path: "sh",
			Args: []string{"-c", `echo "replaced dir=$(pwd -P) foo=$FOO"`},
			Env:  []string{"FOO=bar", "PATH=" + os.Getenv("PATH")},
			Dir:  os.Getenv("AGX_TEST_EXEC_DIR"),
		})
		// Only reached when exec failed.
		// Exit code 3 is the signal; the message is a best-effort hint.
		_, _ = os.Stderr.WriteString("exec failed: " + err.Error())
		os.Exit(3)
	}
	t.Parallel()
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestExecReplace$")
	cmd.Env = append(os.Environ(), execHelperEnv+"=1", "AGX_TEST_EXEC_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	want, err := filepath.EvalSymlinks(dir) // macOS: /var → /private/var; pwd -P prints the real path
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "replaced dir="+want+" foo=bar" {
		t.Errorf("exec output = %q, want dir %s and foo=bar", got, want)
	}
	if err := execReplace(provider.Command{Path: "definitely-not-a-binary-xyz"}); err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("missing binary = %v", err)
	}
}
