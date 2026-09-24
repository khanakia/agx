package cli

import (
	"github.com/spf13/cobra"

	"github.com/khanakia/agx/internal/shellinit"
)

func newShellInitCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "shell-init [zsh|bash]",
		Short: "Print the shell layer (lets new/resume cd, adds your aliases)",
		Long: "Prints shell code that defines an `agx` function — so `agx new` / `agx resume` leave your\n" +
			"shell in the session folder — plus one function per shell.aliases entry in the config.\n" +
			"Add this line to ~/.zshrc (or your shell rc):\n\n" +
			"  eval \"$(agx shell-init zsh)\"",
		Args:      usageArgs(cobra.MaximumNArgs(1)),
		ValidArgs: []string{string(shellinit.Zsh), string(shellinit.Bash)},
		RunE: func(cmd *cobra.Command, args []string) error {
			sh := shellinit.Zsh
			if len(args) == 1 {
				sh = shellinit.Shell(args[0])
			}
			cfg, err := a.config()
			if err != nil {
				return err
			}
			aliases := make([]shellinit.Alias, 0, len(cfg.Aliases))
			for _, al := range cfg.Aliases {
				aliases = append(aliases, shellinit.Alias{Name: al.Name, Args: al.Args})
			}
			if err := shellinit.Write(a.Stdout, shellinit.Params{Shell: sh, Aliases: aliases}); err != nil {
				return usageErr(err)
			}
			return nil
		},
	}
}
