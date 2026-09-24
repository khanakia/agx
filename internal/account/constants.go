// Package account locates Claude Code accounts on this machine: which config
// directories exist, which OAuth access token each one is logged in with,
// and which email / plan it belongs to.
//
// Claude Code keeps one login per config directory. The default is
// ~/.claude; others are selected with CLAUDE_CONFIG_DIR (e.g. ~/.claude-work).
// This package only READS those logins. It never refreshes or writes a
// token, because a refresh rotates the refresh token and would log the
// owning Claude Code install out.
package account

// Rule: NO BARE STRINGS for any on-disk name or keychain service Claude Code
// defines. These are Claude Code's own conventions, verified against
// v2.1.281; if a future version renames one, it changes here only.

const (
	// defaultDirName is the config directory used when CLAUDE_CONFIG_DIR is unset.
	defaultDirName = ".claude"
	// extraDirGlob matches additional config directories (~/.claude-work, …).
	extraDirGlob = ".claude-*"

	// credentialsFile holds the OAuth token as JSON when Claude Code stores it
	// on disk instead of (or as well as) the keychain.
	credentialsFile = ".credentials.json"
	// profileFile holds account metadata (oauthAccount.emailAddress …).
	// For the default dir it lives one level up, at ~/.claude.json.
	profileFile = ".claude.json"

	// keychainService is the keychain service name. With CLAUDE_CONFIG_DIR
	// set, Claude Code (v2.1.52+) appends "-" + the first keychainHashLen hex
	// chars of sha256(config dir path); the bare name is the pre-2.1.52 /
	// default-dir entry.
	keychainService = "Claude Code-credentials"
	keychainHashLen = 8
)

// configMarkers are entries whose presence makes a ~/.claude-* directory a
// real Claude Code config dir. They exclude look-alikes such as
// ~/.claude-worktrees (git worktrees, not an account).
var configMarkers = []string{"settings.json", "projects", credentialsFile, profileFile}

// Credential sources, recorded on Credential.Source for diagnostics.
const (
	SourceFile     = "file"
	SourceKeychain = "keychain"
)
