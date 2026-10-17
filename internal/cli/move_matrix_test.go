package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSessionsMove_SourceRefusals extends TestSessionsMove_Refusals with
// the --from side: an unknown or odd --from name, and a target that is the
// provider's only home (a profile sharing it, with a trailing slash, is
// still the same home). Nothing moves in any case.
func TestSessionsMove_SourceRefusals(t *testing.T) {
	t.Parallel()
	h, d, project := moveHarness(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown --from", []string{"--from", "nobody", "--to", "work"}, `unknown profile "nobody"`},
		{"unicode --from", []string{"--from", "wörk", "--to", "work"}, `unknown profile "wörk"`},
		{"unicode --to", []string{"--to", "wörk"}, `unknown profile "wörk"`},
		{"--from equals --to", []string{"--from", "work", "--to", "work"}, "same home"},
	} {
		code := runWith(h, d, project, append([]string{"sessions", "move"}, tc.args...)...)
		if code != ExitUsage || !strings.Contains(h.stderr.String(), tc.want) {
			t.Errorf("%s: exit %d, stderr %q", tc.name, code, h.stderr.String())
		}
	}

	raw := fmt.Sprintf("sessions:\n  root: %s\nprofiles:\n  - {name: personal, provider: claude, home: %s/.claude, default: true}\n  - {name: kimi, provider: claude, home: %s/.claude/, billing: api}\n", h.sessions, h.home, h.home)
	if err := os.WriteFile(h.cfgPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runWith(h, d, project, "sessions", "move", "--to", "personal"); code != ExitUsage || !strings.Contains(h.stderr.String(), "personal is the only claude account") {
		t.Errorf("only account: exit %d, stderr %q", code, h.stderr.String())
	}
	if _, err := os.Stat(filepath.Join(h.home, ".claude-work")); err == nil {
		t.Error("a refused move created the target home")
	}
}
