package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/status"
	"github.com/spf13/cobra"
)

func (a *app) statusCmd() *cobra.Command {
	var only string
	c := &cobra.Command{
		Use:   "status [profile]",
		Short: "Verify sessions online via sts get-caller-identity",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := a.loadProfiles()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				only = args[0]
			}
			if only != "" {
				p, ok := profiles.Find(list, only)
				if !ok {
					return fmt.Errorf("profile %q not found", only)
				}
				list = []profiles.Profile{p}
			}

			checker := status.NewChecker(a.runner)
			results := checker.CheckAll(cmd.Context(), list)

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "PROFILE\tTYPE\tSTATE\tACCOUNT\tDETAIL")
			for _, p := range list {
				st := results[p.Name]
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					p.Name, p.Type, st.State, dash(st.AccountID), dash(st.Detail))
			}
			return w.Flush()
		},
	}
	return c
}
