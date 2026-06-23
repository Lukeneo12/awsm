package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/spf13/cobra"
)

func (a *app) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List discovered profiles and their type (no network calls)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := a.loadProfiles()
			if err != nil {
				return err
			}
			if len(list) == 0 {
				cmd.PrintErrf("No profiles found in %s or %s\n", a.paths.Config, a.paths.Credentials)
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "PROFILE\tTYPE\tREGION\tDETAIL")
			for _, p := range list {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Type, dash(p.Region), detail(p))
			}
			return w.Flush()
		},
	}
}

func detail(p profiles.Profile) string {
	switch p.Type {
	case profiles.TypeSSO:
		return "sso-session=" + dash(p.SSOSession)
	case profiles.TypeSAML:
		return "saml2aws=" + p.SAMLAccount
	case profiles.TypeRole:
		return "source=" + dash(p.SourceProfile)
	case profiles.TypeManual:
		return "key=" + p.AccessKeyIDMasked
	default:
		return ""
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
