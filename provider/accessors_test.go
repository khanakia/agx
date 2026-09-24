package provider_test

import (
	"testing"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
	"github.com/khanakia/agx/provider/codex"
)

// TestProviderAccessors pins each provider's identity: the id is the config
// key, the binary is what gets exec'd, the default home decides whether the
// home env var is set.
func TestProviderAccessors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		p       provider.Provider
		id      provider.ID
		bin     string
		defHome string
	}{
		{claude.New("/u"), provider.Claude, "claude", "/u/.claude"},
		{codex.New("/u"), provider.Codex, "codex", "/u/.codex"},
	} {
		if tc.p.ID() != tc.id || tc.p.Binary() != tc.bin || tc.p.DefaultHome("/u") != tc.defHome {
			t.Errorf("%T: id=%s bin=%s home=%s", tc.p, tc.p.ID(), tc.p.Binary(), tc.p.DefaultHome("/u"))
		}
	}
}
