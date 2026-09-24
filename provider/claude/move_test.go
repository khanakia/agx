package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// moveFixture lays out a personal home with two conversations for dir,
// their side data, project memory, and one unknown entry.
func moveFixture(t *testing.T) (from, to, dir string) {
	t.Helper()
	root := t.TempDir()
	from, to, dir = filepath.Join(root, ".claude"), filepath.Join(root, ".claude-work"), "/w/docker_setup_mac"
	proj := HistoryDir(from, dir)
	writeFile(t, filepath.Join(proj, "aaaa-1111.jsonl"), `{"type":"attachment","cwd":"/w/docker_setup_mac"}`)
	writeFile(t, filepath.Join(proj, "aaaa-1111", "tool-results", "r.txt"), "R")
	writeFile(t, filepath.Join(proj, "bbbb-2222.jsonl"), `{"type":"attachment","cwd":"/w/docker_setup_mac"}`)
	writeFile(t, filepath.Join(proj, "memory", "note.md"), "M")
	writeFile(t, filepath.Join(proj, "unknown.bin"), "?")
	writeFile(t, filepath.Join(from, fileHistoryDir, "aaaa-1111", "f.txt"), "F")
	writeFile(t, filepath.Join(from, sessionEnvDir, "aaaa-1111", "e"), "E")
	writeFile(t, filepath.Join(from, sessionEnvDir, "bbbb-2222", "e"), "E")
	return from, to, dir
}

func itemFroms(plan provider.MovePlan) []string {
	var out []string
	for _, it := range plan.Items {
		out = append(out, it.From)
	}
	return out
}

func TestPlanMove_Full(t *testing.T) {
	t.Parallel()
	from, to, dir := moveFixture(t)
	plan, err := (&Provider{}).PlanMove(context.Background(), provider.MoveRequest{FromHome: from, ToHome: to, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(plan.Conversations, []string{"aaaa-1111", "bbbb-2222"}) || len(plan.Blockers) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	proj := HistoryDir(from, dir)
	want := []string{
		filepath.Join(proj, "aaaa-1111.jsonl"), filepath.Join(proj, "aaaa-1111"),
		filepath.Join(from, fileHistoryDir, "aaaa-1111"), filepath.Join(from, sessionEnvDir, "aaaa-1111"),
		filepath.Join(proj, "bbbb-2222.jsonl"), filepath.Join(from, sessionEnvDir, "bbbb-2222"),
		filepath.Join(proj, "memory", "note.md"),
	}
	if got := itemFroms(plan); !slices.Equal(got, want) {
		t.Errorf("items =\n%v\nwant\n%v", got, want)
	}
	for _, it := range plan.Items {
		rel, _ := filepath.Rel(from, it.From)
		if it.To != filepath.Join(to, rel) {
			t.Errorf("%s → %s, want mirrored path under the target home", it.From, it.To)
		}
	}
	if len(plan.Left) != 1 || !strings.HasSuffix(plan.Left[0], "unknown.bin") {
		t.Errorf("left = %v (unknown entries must be reported, not dropped)", plan.Left)
	}
	if !slices.Equal(plan.Cleanup, []string{filepath.Join(proj, "memory"), proj}) {
		t.Errorf("cleanup = %v, want memory then the project folder", plan.Cleanup)
	}
}

func TestPlanMove_OnlyIDs(t *testing.T) {
	t.Parallel()
	from, to, dir := moveFixture(t)
	p := &Provider{}
	plan, err := p.PlanMove(context.Background(), provider.MoveRequest{FromHome: from, ToHome: to, Dir: dir, OnlyIDs: []string{"bbbb"}})
	if err != nil || !slices.Equal(plan.Conversations, []string{"bbbb-2222"}) {
		t.Fatalf("prefix pick = %+v, %v", plan.Conversations, err)
	}
	for _, it := range plan.Items {
		if strings.Contains(it.From, "memory") || strings.Contains(it.From, "aaaa") {
			t.Errorf("partial move included %s", it.From)
		}
	}
	if len(plan.Cleanup) != 0 {
		t.Errorf("a partial move must not clean up the folder: %v", plan.Cleanup)
	}
	if _, err := p.PlanMove(context.Background(), provider.MoveRequest{FromHome: from, ToHome: to, Dir: dir, OnlyIDs: []string{"zzzz"}}); err == nil {
		t.Error("unknown id must fail")
	}
	writeFile(t, filepath.Join(HistoryDir(from, dir), "aaaa-3333.jsonl"), "{}")
	if _, err := p.PlanMove(context.Background(), provider.MoveRequest{FromHome: from, ToHome: to, Dir: dir, OnlyIDs: []string{"aaaa"}}); err == nil {
		t.Error("ambiguous prefix must fail")
	}
}

func TestPlanMove_Blockers(t *testing.T) {
	t.Parallel()
	from, to, dir := moveFixture(t)
	// An id already in the target account.
	writeFile(t, filepath.Join(HistoryDir(to, dir), "aaaa-1111.jsonl"), "{}")
	// A live Claude process (this test's own pid) running in the folder,
	// registered with a numeric pid; and a dead one that must be ignored.
	writeFile(t, filepath.Join(to, liveSessionsDir, "live.json"), fmt.Sprintf(`{"pid":%d,"sessionId":"other","cwd":%q}`, os.Getpid(), dir))
	writeFile(t, filepath.Join(from, liveSessionsDir, "dead.json"), fmt.Sprintf(`{"pid":%d,"sessionId":"x","cwd":%q}`, 2147483000, dir))
	plan, err := (&Provider{}).PlanMove(context.Background(), provider.MoveRequest{FromHome: from, ToHome: to, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Blockers, " | ")
	if len(plan.Blockers) != 2 || !strings.Contains(joined, "already exists") || !strings.Contains(joined, fmt.Sprintf("pid %d", os.Getpid())) {
		t.Errorf("blockers = %v", plan.Blockers)
	}
	if err := provider.ApplyMovePlan(plan, ""); err == nil {
		t.Error("a blocked plan must not apply")
	}
}

func TestLiveSessions_PIDForms(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, liveSessionsDir, "a.json"), fmt.Sprintf(`{"pid":%d,"cwd":"/a"}`, os.Getpid()))
	writeFile(t, filepath.Join(home, liveSessionsDir, "b.json"), fmt.Sprintf(`{"pid":"%d","cwd":"/b"}`, os.Getpid()))
	writeFile(t, filepath.Join(home, liveSessionsDir, "c.json"), `not json`)
	writeFile(t, filepath.Join(home, liveSessionsDir, "d.key"), `ignored`)
	got := liveSessions(home)
	if len(got) != 2 {
		t.Errorf("live sessions = %+v, want the numeric and the string pid", got)
	}
	if liveSessions(filepath.Join(home, "missing")) != nil {
		t.Error("missing registry must be empty")
	}
	if processAlive(0) || processAlive(-1) {
		t.Error("non-positive pids are never alive")
	}
}

func TestPlanMove_NoHistory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	plan, err := (&Provider{}).PlanMove(context.Background(), provider.MoveRequest{FromHome: filepath.Join(root, "a"), ToHome: filepath.Join(root, "b"), Dir: "/nothing"})
	if err != nil || len(plan.Conversations) != 0 || len(plan.Items) != 0 {
		t.Errorf("empty = %+v, %v", plan, err)
	}
}
