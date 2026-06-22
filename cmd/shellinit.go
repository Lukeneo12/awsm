package cmd

import (
	"fmt"

	"github.com/Lukeneo12/awsm/internal/shell"
	"github.com/spf13/cobra"
)

func (a *app) shellInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "shell-init <shell>",
		Short:     "Print the shell wrapper function to install (zsh|bash|fish)",
		Long:      "Add the wrapper to your rc file, e.g.:\n  echo 'eval \"$(awsm shell-init zsh)\"' >> ~/.zshrc\nThe wrapper lets `awsm switch` change AWS_PROFILE in your current shell.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: shell.SupportedShells,
		RunE: func(cmd *cobra.Command, args []string) error {
			wrapper, err := shell.Wrapper(args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), wrapper)
			return nil
		},
	}
}
