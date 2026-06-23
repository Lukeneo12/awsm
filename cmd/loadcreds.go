package cmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/Lukeneo12/awsm/internal/creds"
	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/prompt"
	"github.com/spf13/cobra"
)

func (a *app) loadCredsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "load-credentials <profile>",
		Aliases: []string{"load"},
		Short:   "Load manual credentials for a profile by pasting them in the terminal",
		Long: "load-credentials reads an AWS credentials block from stdin, auto-detects the\n" +
			"format (export, ini, PowerShell or cmd), shows a masked preview and stores it\n" +
			"into the profile in ~/.aws/credentials (mode 0600), pinning its type as manual.\n" +
			"Paste the block AWS gives you, then press " + prompt.EOFKey + ". You can also pipe\n" +
			"it in: awsm load <profile> < creds.txt. The secret is never printed.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := args[0]

			stderrf("Paste the AWS credentials block, then press %s:\n", prompt.EOFKey)
			// Bound the read: a pasted credentials block is a few hundred bytes;
			// 1 MiB is generous and guards against a runaway pipe.
			raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
			if err != nil {
				return fmt.Errorf("reading pasted credentials: %w", err)
			}
			parsed, err := creds.Parse(string(raw))
			if err != nil {
				return fmt.Errorf("%w (paste an AWS credentials block, then press %s)", err, prompt.EOFKey)
			}

			kind := "long-term"
			if parsed.SessionToken != "" {
				kind = "temporary"
			}
			region := parsed.Region
			if region == "" {
				region = "(none)"
			}
			stderrf("About to load into %q:\n", profile)
			stderrf("  key:    ****%s\n", last4(parsed.AccessKeyID))
			stderrf("  region: %s\n", region)
			stderrf("  type:   %s\n", kind)

			ok, err := a.confirm("Save?")
			switch {
			case errors.Is(err, prompt.ErrNoTTY):
				stderrf("non-interactive: saved without confirmation\n")
				ok = true
			case err != nil:
				return err
			}
			if !ok {
				stderrf("aborted, nothing written\n")
				return nil
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

			stderrf("loaded %s credentials into %q (key ****%s) [mode 0600]\n",
				kind, profile, last4(parsed.AccessKeyID))
			return nil
		},
	}
}
