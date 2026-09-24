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
