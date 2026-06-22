package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/shell"
	"github.com/spf13/cobra"
)

func (a *app) switchCmd() *cobra.Command {
	var shellName string
	c := &cobra.Command{
		Use:   "switch <profile>",
		Short: "Print the eval snippet that makes a profile active in the current shell",
		Long: "switch prints an `export AWS_PROFILE=...` snippet to stdout. It only takes\n" +
			"effect when wrapped by the shell function from `awsm shell-init` (which evals\n" +
			"this output). Diagnostics go to stderr so they never break the eval.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := a.loadProfiles()
			if err != nil {
				return err
			}
			p, ok := profiles.Find(list, args[0])
			if !ok {
				return fmt.Errorf("profile %q not found (run `awsm list`)", args[0])
			}
			if shellName == "" {
				shellName = detectShell()
			}
			fmt.Fprint(cmd.OutOrStdout(), shell.ExportSnippet(p, shellName))
			stderrf("switched to %s\n", p.Name)
			return nil
		},
	}
	c.Flags().StringVar(&shellName, "shell", "", "target shell for the snippet (zsh|bash|fish); autodetected from $SHELL")
	return c
}

// detectShell guesses the user's shell from $SHELL, defaulting to posix export.
func detectShell() string {
	base := filepath.Base(os.Getenv("SHELL"))
	switch base {
	case "fish":
		return "fish"
	default:
		return "bash" // bash/zsh share `export`
	}
}
