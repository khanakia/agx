package claude

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

// newTestProvider returns a Provider over a temp user home with the given
// stores, and a usage server answering with the fixture.
func newTestProvider(t *testing.T, now time.Time) (*Provider, string) {
	t.Helper()
	home := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(loadFixture(t)) // test server
	}))
	t.Cleanup(srv.Close)
	return &Provider{
		UserHome:    home,
		Store:       fakeStore{secrets: map[string]string{}, errs: map[string]error{}},
		UsageClient: &UsageClient{Endpoint: srv.URL, HTTP: srv.Client()},
		Now:         func() time.Time { return now },
	}, home
}

func TestProvider_UsageStates(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	p, home := newTestProvider(t, now)

	work := filepath.Join(home, ".claude-work")
	expired := filepath.Join(home, ".claude-old")
	loggedOut := filepath.Join(home, ".claude-none")
	writeFile(t, filepath.Join(work, credentialsFile), credJSON("ok", now.Add(time.Hour).UnixMilli()))
	writeFile(t, filepath.Join(expired, credentialsFile), credJSON("old", now.Add(-time.Hour).UnixMilli()))
	if err := os.MkdirAll(loggedOut, 0o700); err != nil {
		t.Fatal(err)
	}

	u, err := p.Usage(context.Background(), provider.Profile{Home: work, Billing: provider.BillingPlan})
	if err != nil || len(u.Windows) != 3 || u.Identity.Plan != "Max (20x)" {
		t.Errorf("work usage = %+v, %v", u, err)
	}
	if _, err := p.Usage(context.Background(), provider.Profile{Home: expired}); !errors.Is(err, provider.ErrExpired) {
		t.Errorf("expired err = %v", err)
	}
	if _, err := p.Usage(context.Background(), provider.Profile{Home: loggedOut}); !errors.Is(err, provider.ErrNotLoggedIn) {
		t.Errorf("logged-out err = %v", err)
	}
	if _, err := p.Usage(context.Background(), provider.Profile{Home: work, Billing: provider.BillingAPI}); !errors.Is(err, provider.ErrAPIBilled) {
		t.Errorf("api-billed err = %v", err)
	}
}

func TestProvider_Launch(t *testing.T) {
	t.Parallel()
	p := &Provider{UserHome: "/u"}
	base := []string{"PATH=/bin", ConfigDirEnv + "=/stale"}

	// Non-default home: env var set, profile args first, resume, then extra args.
	cmd, err := p.Launch(provider.Profile{Home: "/u/.claude-work", Args: []string{"--a"}, Env: map[string]string{"X": "1"}},
		provider.LaunchRequest{Dir: "/d", ResumeID: "sid", Args: []string{"--b"}, Env: base})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != Binary || strings.Join(cmd.Args, " ") != "--a --resume sid --b" || cmd.Dir != "/d" {
		t.Errorf("cmd = %+v", cmd)
	}
	if !slices.Contains(cmd.Env, ConfigDirEnv+"=/u/.claude-work") || !slices.Contains(cmd.Env, "X=1") || slices.Contains(cmd.Env, ConfigDirEnv+"=/stale") {
		t.Errorf("env = %v", cmd.Env)
	}
	if len(base) != 2 || base[1] != ConfigDirEnv+"=/stale" {
		t.Error("Launch mutated the caller's env slice")
	}

	// Default home: the variable must be removed, even if the shell set it.
	cmd, _ = p.Launch(provider.Profile{Home: "/u/.claude"}, provider.LaunchRequest{Env: base})
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, ConfigDirEnv+"=") {
			t.Errorf("default home kept %s", kv)
		}
	}
}

func TestProvider_Discover(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), "{}")
	writeFile(t, filepath.Join(home, ".claude-work", "settings.json"), "{}")
	ps, err := (&Provider{}).Discover(home)
	if err != nil || len(ps) != 2 {
		t.Fatalf("Discover = %v, %v", ps, err)
	}
	if ps[0].Name != "personal" || !ps[0].Default || ps[1].Name != "work" || ps[1].Default || ps[1].Source != provider.SourceDiscovered {
		t.Errorf("profiles = %+v", ps)
	}
}

func TestEncodePath(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"/Users/you/work/sessions/20260923_175923": "-Users-you-work-sessions-20260923-175923",
		"/a/chat_go copy":                          "-a-chat-go-copy",
		"/x/v1.2/ü":                                "-x-v1-2---", // multi-byte runes: one dash per byte
	} {
		if got := EncodePath(in); got != want {
			t.Errorf("EncodePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// writeTranscript writes a minimal Claude transcript.
func writeTranscript(t *testing.T, home, cwd, id string, lines ...string) string {
	t.Helper()
	body := `{"type":"mode","sessionId":"` + id + `"}` + "\n" +
		`{"type":"attachment","cwd":"` + cwd + `","sessionId":"` + id + `"}` + "\n" +
		strings.Join(lines, "\n") + "\n"
	path := filepath.Join(HistoryDir(home, cwd), id+transcriptExt)
	writeFile(t, path, body)
	return path
}

func TestConversations(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	older := writeTranscript(t, home, "/w/a", "id-a",
		`{"type":"ai-title","aiTitle":"First title"}`,
		`{"type":"ai-title","aiTitle":"Newest AI title"}`)
	writeTranscript(t, home, "/w/b", "id-b",
		`{"type":"ai-title","aiTitle":"Generated"}`,
		`{"type":"custom-title","customTitle":"User named"}`)
	writeTranscript(t, home, "/w/a_b", "id-collide") // encodes like "/w/a-b"
	// A transcript with no cwd cannot be resumed and is skipped.
	writeFile(t, filepath.Join(home, projectsDir, "-junk", "id-junk.jsonl"), `{"type":"mode"}`+"\n")

	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, old, old); err != nil {
		t.Fatal(err)
	}

	p := &Provider{}
	all, err := p.Conversations(context.Background(), home, provider.ConversationQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d conversations: %+v", len(all), all)
	}
	byID := map[string]provider.Conversation{}
	for _, c := range all {
		byID[c.ID] = c
	}
	if byID["id-a"].Title != "Newest AI title" || byID["id-b"].Title != "User named" || byID["id-a"].Dir != "/w/a" {
		t.Errorf("titles/dirs = %+v", byID)
	}
	if all[len(all)-1].ID != "id-a" {
		t.Errorf("not newest-first: %+v", all)
	}

	scoped, _ := p.Conversations(context.Background(), home, provider.ConversationQuery{Dir: "/w/a"})
	if len(scoped) != 1 || scoped[0].ID != "id-a" {
		t.Errorf("scoped = %+v", scoped)
	}
	limited, _ := p.Conversations(context.Background(), home, provider.ConversationQuery{Limit: 1})
	if len(limited) != 1 {
		t.Errorf("limit ignored: %d", len(limited))
	}
	none, err := p.Conversations(context.Background(), t.TempDir(), provider.ConversationQuery{})
	if err != nil || len(none) != 0 {
		t.Errorf("empty home = %v, %v", none, err)
	}
}

// TestConversations_AfterMove pins the promote case: history re-keyed to a
// new folder still carries the OLD cwd inside the transcript, and a scoped
// lookup from the new folder must find it and report the new folder.
func TestConversations_AfterMove(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeTranscript(t, home, "/s/old", "id-1", `{"type":"ai-title","aiTitle":"Moved chat"}`)
	p := &Provider{}
	if _, err := p.MoveHistory(home, "/s/old", "/p/new"); err != nil {
		t.Fatal(err)
	}
	got, err := p.Conversations(context.Background(), home, provider.ConversationQuery{Dir: "/p/new"})
	if err != nil || len(got) != 1 || got[0].ID != "id-1" || got[0].Dir != "/p/new" || got[0].Title != "Moved chat" {
		t.Fatalf("after move = %+v, %v", got, err)
	}
	if old, _ := p.Conversations(context.Background(), home, provider.ConversationQuery{Dir: "/s/old"}); len(old) != 0 {
		t.Errorf("old folder still lists %+v", old)
	}
}

func TestMoveHistoryAndHasHistory(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeTranscript(t, home, "/s/old", "id-1")
	p := &Provider{}

	if !p.HasHistory(home, "/s/old") || p.HasHistory(home, "/s/new") {
		t.Fatal("HasHistory before move wrong")
	}
	moved, err := p.MoveHistory(home, "/s/old", "/p/new")
	if err != nil || !moved {
		t.Fatalf("MoveHistory = %v, %v", moved, err)
	}
	if p.HasHistory(home, "/s/old") || !p.HasHistory(home, "/p/new") {
		t.Error("history not re-keyed")
	}
	// Nothing to move is not an error.
	if moved, err := p.MoveHistory(home, "/s/none", "/p/x"); err != nil || moved {
		t.Errorf("empty move = %v, %v", moved, err)
	}
	// Never overwrite an existing destination.
	writeTranscript(t, home, "/s/two", "id-2")
	if _, err := p.MoveHistory(home, "/s/two", "/p/new"); !errors.Is(err, ErrHistoryExists) {
		t.Errorf("overwrite err = %v", err)
	}
}
