package cli

import (
	"github.com/khanakia/voltkit/versioncmd"
	"github.com/spf13/cobra"

	"github.com/khanakia/agx/internal/appmeta"
)

// Flag names shared by several commands.
const (
	flagJSON     = "json"
	flagProfile  = "profile"
	flagProvider = "provider"
	flagColor    = "color"
	flagTimeout  = "timeout"
	flagDryRun   = "dry-run"
)

// Command groups for `agx --help`.
const (
	groupAgents   = "agents"
	groupSessions = "sessions"
	groupSetup    = "setup"
)

// NewRoot builds the command tree for one invocation.
func NewRoot(d Deps) *cobra.Command {
	a := &app{Deps: d}
	root := &cobra.Command{
		Use:   appmeta.Name,
		Short: "One CLI for your AI coding agents and all their accounts",
		Long: appmeta.Name + " drives your AI coding agents (Claude Code, Codex) across every account on this machine:\n" +
			"plan usage for all accounts at once, launching on the account with the most headroom,\n" +
			"resuming a conversation on the account that holds it, and managing session folders.\n\n" +
			"Run with no command to show usage for every account.",
		Version:       versioncmd.Collect(appmeta.Name).Version,
		Args:          usageArgs(cobra.NoArgs),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageErr(err) })
	root.AddGroup(
		&cobra.Group{ID: groupAgents, Title: "Agents:"},
		&cobra.Group{ID: groupSessions, Title: "Sessions:"},
		&cobra.Group{ID: groupSetup, Title: "Setup:"},
	)

	usage := newUsageCmd(a)
	// The bare command shows usage, sharing the usage command's flags.
	root.Flags().AddFlagSet(usage.Flags())
	root.RunE = usage.RunE

	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add(groupAgents, usage, newProfilesCmd(a), newRunCmd(a), newNewCmd(a), newResumeCmd(a))
	add(groupSessions, newSessionsCmd(a))
	add(groupSetup, newDoctorCmd(a), newShellInitCmd(a),
		versioncmd.New(versioncmd.WithBinaryName(appmeta.Name)))
	root.AddCommand(newExecCmd(a)) // hidden plumbing for the shell layer
	return root
}

// usageArgs wraps a cobra argument validator so its failures exit 2.
func usageArgs(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			return usageErr(err)
		}
		return nil
	}
}
