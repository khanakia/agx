module github.com/khanakia/agx

go 1.26.4

require (
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.9
	github.com/ubgo/voltkit/appdir v0.2.0
	github.com/ubgo/voltkit/output v0.2.0
	github.com/ubgo/voltkit/versioncmd v0.2.1
	golang.org/x/term v0.45.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ubgo/buildinfo v0.1.2 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

// v0.2.0 read a stale "Claude Code-credentials" keychain entry and reported
// a logged-in account as expired; fixed in v0.2.1.
retract v0.2.0
