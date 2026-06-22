package cmd

import (
	"fmt"

	"github.com/Lukeneo12/awsm/internal/clipboard"
	"github.com/Lukeneo12/awsm/internal/creds"
	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/spf13/cobra"
)

func (a *app) loadCredsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "load-credentials <profile>",
		Aliases: []string{"load"},
		Short:   "Load manual credentials for a profile from the clipboard",
		Long: "load-credentials reads the clipboard, auto-detects the format (export, ini,\n" +
			"PowerShell or cmd) and stores the credentials into the profile in\n" +
			"~/.aws/credentials (mode 0600), pinning its type as manual. Paste the block\n" +
			"AWS gives you (e.g. the SSO portal) first, then run this. The secret is never\n" +
			"printed.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := args[0]

			text, err := clipboard.Read(a.runner)
			if err != nil {
				return err
			}
			parsed, err := creds.Parse(text)
			if err != nil {
				return fmt.Errorf("%w (copy an AWS credentials block to the clipboard first)", err)
			}

			in := profiles.ManualInput{
				AccessKeyID:  parsed.AccessKeyID,
				Secret:       parsed.SecretAccessKey,
				SessionToken: parsed.SessionToken,
				Region:       parsed.Region,
			}
			if err := profiles.AddManual(a.paths.Credentials, a.paths.Config, profile, in); err != nil {
				return err
			}
			_ = profiles.SetOverride(a.paths.Overrides, profile, profiles.Override{Type: profiles.TypeManual})

			kind := "long-term"
			if parsed.SessionToken != "" {
				kind = "temporary"
			}
			stderrf("loaded %s credentials into %q (key ****%s) [mode 0600]\n",
				kind, profile, last4(parsed.AccessKeyID))
			return nil
		},
	}
}
