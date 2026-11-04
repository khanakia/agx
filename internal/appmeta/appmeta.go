// Package appmeta is agx's identity: the one place its name, environment
// prefix and dot-dir are decided. Library packages never hardcode these; the
// CLI passes them in (voltkit's "library owns behaviour, app owns identity").
package appmeta

// Identity values.
const (
	// Name is the binary and command name.
	Name = "agx"
	// EnvPrefix prefixes every agx environment variable (AGX_HOME,
	// AGX_CONFIG_DIR, AGX_PLAN_FILE, …).
	EnvPrefix = "AGX"
	// DirName is the home dot-dir holding agx's config (~/.agx).
	DirName = ".agx"
	// RepoURL is where users report problems.
	RepoURL = "https://github.com/khanakia/agx"
)

// Environment variables agx reads besides appdir's own AGX_HOME /
// AGX_<CATEGORY>_DIR.
const (
	// EnvPlanFile, when set, makes `new` / `resume` write a launch plan to
	// this path instead of launching (spec §10).
	EnvPlanFile = EnvPrefix + "_PLAN_FILE"
	// EnvShellInit is exported by the generated shell layer so `agx doctor`
	// can tell whether it is active.
	EnvShellInit = EnvPrefix + "_SHELL_INIT"
)

// EnvLogin chooses the login for every run in one terminal (logins spec
// §5.3); set by the `aglogin` shell function.
const EnvLogin = EnvPrefix + "_LOGIN"

// Variables agx SETS on every agent it launches so the running session (or a
// status line, or `agx whoami` inside it) can tell what launched it. They are
// outputs only: agx never reads them to choose a profile or login, so a value
// inherited from an outer session can never steer a nested launch, and each
// launch overwrites whatever the parent had. They carry names and a path,
// never a token.
const (
	// EnvRunProfile is the profile name the session was launched on.
	EnvRunProfile = EnvPrefix + "_RUN_PROFILE"
	// EnvRunLogin is the login the session spends: a login name, or "home"
	// when the home's own login is used.
	EnvRunLogin = EnvPrefix + "_RUN_LOGIN"
	// EnvRunHome is the profile's home directory. It is set even for the
	// vendor's default home, where the vendor's own variable
	// (CLAUDE_CONFIG_DIR, CODEX_HOME) must stay absent.
	EnvRunHome = EnvPrefix + "_RUN_HOME"
)
