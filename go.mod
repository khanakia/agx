module github.com/khanakia/agx

go 1.26.4

require (
	github.com/khanakia/voltkit/appdir v0.1.0
	github.com/khanakia/voltkit/output v0.1.0
	github.com/khanakia/voltkit/versioncmd v0.1.0
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/ubgo/buildinfo v0.1.2 // indirect
)

// v0.2.0 read a stale "Claude Code-credentials" keychain entry and reported
// a logged-in account as expired; fixed in v0.2.1.
retract v0.2.0
