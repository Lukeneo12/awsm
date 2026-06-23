package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/spf13/cobra"
)

func (a *app) setTypeCmd() *cobra.Command {
	var account string
	var clear bool
	c := &cobra.Command{
		Use:   "set-type <profile> [manual|sso|saml|role]",
		Short: "Pin a profile's type, overriding auto-detection",
		Long: "set-type fixes how awsm classifies a profile when the files are ambiguous\n" +
			"(e.g. a profile that has a saml2aws entry but is filled by hand). Pass the type\n" +
			"as the second argument, or omit it to be prompted. Use --clear to remove the\n" +
			"override and fall back to auto-detection.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := args[0]

			if clear {
				if err := profiles.SetOverride(a.paths.Overrides, profile, profiles.Override{}); err != nil {
					return err
				}
				cmd.PrintErrf("cleared type override for %q\n", profile)
				return nil
			}

			var t profiles.Type
			if len(args) == 2 {
				t = profiles.Type(strings.ToLower(strings.TrimSpace(args[1])))
			} else {
				cmd.PrintErrf("type for %q [manual/sso/saml/role]: ", profile)
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				t = profiles.Type(strings.ToLower(strings.TrimSpace(line)))
			}
			if !profiles.IsValidType(t) {
				return fmt.Errorf("invalid type %q (want manual|sso|saml|role; or --clear)", t)
			}
			if err := profiles.SetOverride(a.paths.Overrides, profile, profiles.Override{Type: t, Account: account}); err != nil {
				return err
			}
			cmd.PrintErrf("pinned %q as %q\n", profile, t)
			return nil
		},
	}
	c.Flags().StringVar(&account, "account", "", "saml2aws account name (when type is saml)")
	c.Flags().BoolVar(&clear, "clear", false, "remove the override and use auto-detection")
	return c
}
