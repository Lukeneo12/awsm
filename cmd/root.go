// Package cmd wires the awsm CLI (cobra) and dispatches to the internal
// packages. Running awsm with no subcommand launches the TUI.
package cmd

import (
	"fmt"
	"os"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/runner"
	"github.com/Lukeneo12/awsm/internal/tui"
	"github.com/spf13/cobra"
)

// app holds shared dependencies for the commands.
type app struct {
	paths  profiles.Paths
	runner runner.CommandRunner
}

func newApp() *app {
	return &app{paths: profiles.DefaultPaths(), runner: runner.New()}
}

// Execute builds the command tree and runs it.
func Execute() error {
	a := newApp()
	var switchFile string
	root := &cobra.Command{
		Use:           "awsm",
		Short:         "Local manager for AWS credentials (SSO, SAML, assume-role, static keys)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// No subcommand -> launch the TUI.
			list, err := profiles.List(a.paths)
			if err != nil {
				return err
			}
			return tui.Run(a.runner, a.paths, list, switchFile)
		},
	}
	// Hidden flag set by the shell wrapper: the TUI writes the chosen profile
	// here on switch, and the wrapper applies it in the parent shell.
	root.Flags().StringVar(&switchFile, "switch-file", "", "")
	_ = root.Flags().MarkHidden("switch-file")

	root.AddCommand(
		a.listCmd(),
		a.statusCmd(),
		a.switchCmd(),
		a.loginCmd(),
		a.addCmd(),
		a.rmCmd(),
		a.setTypeCmd(),
		a.loadCredsCmd(),
		a.shellInitCmd(),
	)
	return root.Execute()
}

// mustList loads profiles or prints a friendly error.
func (a *app) loadProfiles() ([]profiles.Profile, error) {
	list, err := profiles.List(a.paths)
	if err != nil {
		return nil, fmt.Errorf("reading AWS config: %w", err)
	}
	return list, nil
}

// stderrf prints to stderr so it never pollutes evaluable stdout.
func stderrf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}
