package cmd

import (
	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/spf13/cobra"
)

func (a *app) rmCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rm <profile>",
		Aliases: []string{"remove", "forget"},
		Short:   "Forget a profile: remove it from credentials and config, and clear its override",
		Long: "rm fully forgets a profile: it deletes the section from ~/.aws/credentials and\n" +
			"~/.aws/config and clears any awsm type override. For SSO/role profiles this\n" +
			"removes their config definition too — the type is reported so you know what went.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := args[0]

			// Look up the profile first so we can report what we're removing.
			list, err := a.loadProfiles()
			if err != nil {
				return err
			}
			p, existed := profiles.Find(list, profile)

			if err := profiles.RemoveProfile(a.paths.Credentials, profile); err != nil {
				return err
			}
			if err := profiles.RemoveConfigProfile(a.paths.Config, profile); err != nil {
				return err
			}
			if err := profiles.SetOverride(a.paths.Overrides, profile, profiles.Override{}); err != nil {
				return err
			}

			if existed {
				cmd.PrintErrf("forgot %q (was %s) — removed from credentials & config, override cleared\n", profile, p.Type)
			} else {
				cmd.PrintErrf("nothing to remove for %q\n", profile)
			}
			return nil
		},
	}
}
