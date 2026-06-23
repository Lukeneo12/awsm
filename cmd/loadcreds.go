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

			cmd.PrintErrf("Paste the AWS credentials block, then press %s:\n", prompt.EOFKey)
			// Bound the read: a pasted credentials block is a few hundred bytes;
			// 1 MiB is generous. Read one byte past the cap so we can tell a
			// runaway pipe apart from a legitimately-sized block and fail loudly
			// instead of silently parsing a truncated input.
			const maxCreds = 1 << 20
			raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxCreds+1))
			if err != nil {
				return fmt.Errorf("reading pasted credentials: %w", err)
			}
			if len(raw) > maxCreds {
				return fmt.Errorf("pasted input exceeds %d bytes; refusing to parse a possibly truncated block", maxCreds)
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
			cmd.PrintErrf("About to load into %q:\n", profile)
			cmd.PrintErrf("  key:    ****%s\n", last4(parsed.AccessKeyID))
			cmd.PrintErrf("  region: %s\n", region)
			cmd.PrintErrf("  type:   %s\n", kind)

			// app is always built via newApp (or the tests), which wire confirm.
			// Guard anyway so a hand-built app degrades to the real console
			// prompt instead of panicking on a nil call.
			confirm := a.confirm
			if confirm == nil {
				confirm = prompt.Confirm
			}
			ok, err := confirm("Save?")
			switch {
			case errors.Is(err, prompt.ErrNoTTY):
				cmd.PrintErrf("non-interactive: saved without confirmation\n")
				ok = true
			case err != nil:
				return err
			}
			if !ok {
				cmd.PrintErrf("aborted, nothing written\n")
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

			cmd.PrintErrf("loaded %s credentials into %q (key ****%s) [mode 0600]\n",
				kind, profile, last4(parsed.AccessKeyID))
			return nil
		},
	}
}
