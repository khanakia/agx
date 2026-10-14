package cli

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/provider"
)

// TestConversationViews pins which profile a listed conversation shows: the
// one resume launches on, every candidate on a shared home, and the home's
// basename when no profile uses it.
func TestConversationViews(t *testing.T) {
	t.Parallel()
	cfg := config.Config{Profiles: []provider.Profile{
		{Name: "personal", Provider: provider.Claude, Home: "/h/.claude", Billing: provider.BillingPlan, Default: true},
		{Name: "work", Provider: provider.Claude, Home: "/h/.claude-work", Billing: provider.BillingPlan},
		{Name: "kimi", Provider: provider.Claude, Home: "/h/.claude", Billing: provider.BillingAPI},
		{Name: "codex", Provider: provider.Codex, Home: "/h/.codex", Billing: provider.BillingPlan, Default: true},
	}}
	tests := []struct {
		name     string
		conv     provider.Conversation
		profile  string
		profiles []string
	}{
		{"unique home", provider.Conversation{Provider: provider.Claude, Home: "/h/.claude-work"}, "work", []string{"work"}},
		{"shared home", provider.Conversation{Provider: provider.Claude, Home: "/h/.claude"}, "personal", []string{"personal", "kimi"}},
		{"codex", provider.Conversation{Provider: provider.Codex, Home: "/h/.codex"}, "codex", []string{"codex"}},
		{"codex on a claude home", provider.Conversation{Provider: provider.Codex, Home: "/h/.claude"}, ".claude", nil},
		{"unknown home", provider.Conversation{Provider: provider.Claude, Home: "/h/.claude-old"}, ".claude-old", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := conversationViews(cfg, []provider.Conversation{tt.conv})[0]
			if v.Profile != tt.profile || !slices.Equal(v.Profiles, tt.profiles) {
				t.Errorf("view = %q %v; want %q %v", v.Profile, v.Profiles, tt.profile, tt.profiles)
			}
			// The listed profile is the one resume launches on.
			if p, ok := cfg.ForHome(tt.conv.Provider, tt.conv.Home); ok && p.Name != v.Profile {
				t.Errorf("list says %q, launch uses %q", v.Profile, p.Name)
			}
		})
	}
}

// TestConversationLine_FitsOneLine checks every row has the same width no
// matter how long the title, folder or profile name is, and that the short id
// is a usable prefix.
func TestConversationLine_FitsOneLine(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	a := &app{Deps: Deps{UserHome: "/home/me", Now: func() time.Time { return now }}}
	long := strings.Repeat("x", 200)
	uuid := "b078a831-c2b0-4921-9b4f-23a7c8f69800"
	tests := []struct {
		name    string
		v       conversationView
		wantDir string
		wantID  string
	}{
		{"short everything", conversationView{Conversation: provider.Conversation{Provider: provider.Codex, ID: "ab", Dir: "/home/me/p", Updated: now}, Profile: "work"}, "~/p", "ab"},
		{"full uuid, untitled", conversationView{Conversation: provider.Conversation{ID: uuid, Dir: "/home/me", Updated: now}, Profile: "personal"}, "~", "b078a831"},
		{"long folder outside home", conversationView{Conversation: provider.Conversation{ID: uuid, Dir: "/Volumes/D/www/projects/khanakia/sessions/20261013_131129", Updated: now}, Profile: "work"}, "…/20261013_131129", "b078a831"},
		{"long everything", conversationView{Conversation: provider.Conversation{ID: uuid, Provider: provider.ID(long), Title: long, Dir: "/srv/" + long, Updated: now.Add(-400 * 24 * time.Hour)}, Profile: long}, "…/xxx", "b078a831"},
		{"unicode title", conversationView{Conversation: provider.Conversation{ID: uuid, Title: strings.Repeat("é", 100), Dir: "/home/me/é", Updated: now}, Profile: "kimi"}, "~/é", "b078a831"},
	}
	wantWidth := agoWidth + 1 + providerWidth + 1 + profileWidth + 1 + titleWidth + 1 + dirWidth + 2 + shortIDLen
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			line := a.conversationLine(tt.v)
			if strings.Contains(line, "\n") {
				t.Fatalf("row has a newline: %q", line)
			}
			if w := utf8.RuneCountInString(line); w > wantWidth {
				t.Errorf("width %d > %d: %q", w, wantWidth, line)
			}
			if !strings.Contains(line, tt.wantDir) || !strings.HasSuffix(line, "  "+tt.wantID) {
				t.Errorf("row %q: want dir %q, id %q", line, tt.wantDir, tt.wantID)
			}
			if tt.v.Provider != "" && !strings.HasPrefix(line[agoWidth+1:], truncate(string(tt.v.Provider), providerWidth)+" ") {
				t.Errorf("provider %q not in its column: %q", tt.v.Provider, line)
			}
			if tt.v.Title == "" && !strings.Contains(line, untitled) {
				t.Errorf("untitled not marked: %q", line)
			}
			if !strings.HasPrefix(tt.v.ID, tt.wantID) {
				t.Errorf("short id %q is not a prefix of %q", tt.wantID, tt.v.ID)
			}
		})
	}
}
