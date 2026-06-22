package cmd

import (
	"fmt"

	"github.com/Lukeneo12/awsm/internal/auth"
	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/spf13/cobra"
)

func (a *app) loginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <profile>",
		Short: "Authenticate a profile (dispatches to aws sso login or saml2aws)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := a.loadProfiles()
			if err != nil {
				return err
			}
			p, ok := profiles.Find(list, args[0])
			if !ok {
				return fmt.Errorf("profile %q not found (run `awsm list`)", args[0])
			}

			plan := auth.PlanFor(p)
			if plan.NoOp {
				stderrf("%s: %s\n", p.Name, plan.Note)
				return nil
			}
			stderrf("logging in %s via %s...\n", p.Name, plan.Binary)

			authn := auth.New(a.runner)
			if err := authn.Login(cmd.Context(), p, list); err != nil {
				return err
			}
			stderrf("done\n")
			return nil
		},
	}
}
