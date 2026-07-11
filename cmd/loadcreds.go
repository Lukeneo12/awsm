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

// maxPastedCreds bounds a pasted credentials block read from stdin. A real
// block is a few hundred bytes; 1 MiB is generous. readPastedBlock reads one
// byte past the cap so it can tell a runaway pipe apart from a legitimately
// sized block and fail loudly instead of silently parsing a truncated input.
const maxPastedCreds = 1 << 20

// readPastedBlock reads an AWS credentials block from in until EOF, bounded at
// maxPastedCreds bytes. Both `load-credentials` and `add --type manual` read
// the whole block in one shot instead of prompting field by field, so neither
// is exposed to term.ReadPassword's single-line truncation of a session token
// that carries an embedded newline (the bug this feature fixes).
func readPastedBlock(in io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(in, maxPastedCreds+1))
	if err != nil {
		return "", fmt.Errorf("reading pasted credentials: %w", err)
	}
	if len(raw) > maxPastedCreds {
		return "", fmt.Errorf("pasted input exceeds %d bytes; refusing to parse a possibly truncated block", maxPastedCreds)
	}
	return string(raw), nil
}

// parsePastedCreds parses a raw pasted block, wrapping a parse failure with
// the same paste hint `load-credentials` has always shown.
func parsePastedCreds(raw string) (creds.Parsed, error) {
	parsed, err := creds.Parse(raw)
	if err != nil {
		return creds.Parsed{}, fmt.Errorf("%w (paste an AWS credentials block, then press %s)", err, prompt.EOFKey)
	}
	return parsed, nil
}

// credsKind classifies parsed credentials for the preview/report lines.
func credsKind(parsed creds.Parsed) string {
	if parsed.SessionToken != "" {
		return "temporary"
	}
	return "long-term"
}

// printCredsPreview shows a masked preview of parsed on errw: the access key
// id's last 4 characters, the region (or "(none)") and whether the
// credentials are temporary or long-term. The secret and session token are
// never included.
func printCredsPreview(errw io.Writer, parsed creds.Parsed) {
	region := parsed.Region
	if region == "" {
		region = "(none)"
	}
	fmt.Fprintf(errw, "  key:    ****%s\n", last4(parsed.AccessKeyID))
	fmt.Fprintf(errw, "  region: %s\n", region)
	fmt.Fprintf(errw, "  type:   %s\n", credsKind(parsed))
}

// confirmPastedCreds asks confirm (falling back to the real console prompt
// when confirm is nil, which happens for a hand-built app outside of
// newApp/tests) whether to save. When no console is available
// (prompt.ErrNoTTY, e.g. a headless CI run) it auto-confirms with a notice
// instead of blocking forever.
func confirmPastedCreds(errw io.Writer, confirm func(string) (bool, error)) (bool, error) {
	if confirm == nil {
		confirm = prompt.Confirm
	}
	ok, err := confirm("Save?")
	switch {
	case errors.Is(err, prompt.ErrNoTTY):
		fmt.Fprintf(errw, "non-interactive: saved without confirmation\n")
		return true, nil
	case err != nil:
		return false, err
	}
	return ok, nil
}

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
			raw, err := readPastedBlock(cmd.InOrStdin())
			if err != nil {
				return err
			}
			parsed, err := parsePastedCreds(raw)
			if err != nil {
				return err
			}

			cmd.PrintErrf("About to load into %q:\n", profile)
			printCredsPreview(cmd.ErrOrStderr(), parsed)

			ok, err := confirmPastedCreds(cmd.ErrOrStderr(), a.confirm)
			if err != nil {
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
				credsKind(parsed), profile, last4(parsed.AccessKeyID))
			return nil
		},
	}
}
