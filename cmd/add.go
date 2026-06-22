package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func (a *app) addCmd() *cobra.Command {
	var typeFlag string
	c := &cobra.Command{
		Use:   "add <profile>",
		Short: "Add or configure a profile (manual | sso | saml | role) — interactive wizard",
		Long: "add walks you through creating a profile of any type, writing the right\n" +
			"files (~/.aws/credentials, ~/.aws/config or ~/.saml2aws) and pinning its type\n" +
			"in ~/.config/awsm/profiles.ini. Secrets are read without echo and never printed.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := newWizard(cmd)

			var profile string
			if len(args) == 1 {
				profile = args[0]
			} else {
				profile = w.ask("profile", "profile name", "")
			}
			if profile == "" {
				return fmt.Errorf("a profile name is required")
			}

			t := profiles.Type(strings.ToLower(strings.TrimSpace(typeFlag)))
			if t == "" {
				t = profiles.Type(w.ask("type", "type [manual/sso/saml/role]", "manual"))
			}
			if !profiles.IsValidType(t) {
				return fmt.Errorf("invalid type %q (want manual|sso|saml|role)", t)
			}

			switch t {
			case profiles.TypeManual:
				return a.addManual(w, profile)
			case profiles.TypeSSO:
				return a.addSSO(w, profile)
			case profiles.TypeSAML:
				return a.addSAML(w, profile)
			case profiles.TypeRole:
				return a.addRole(w, profile)
			}
			return nil
		},
	}
	c.Flags().StringVar(&typeFlag, "type", "", "profile type (manual|sso|saml|role); prompted if omitted")
	return c
}

func (a *app) addManual(w *wizard, profile string) error {
	in := profiles.ManualInput{
		AccessKeyID:  w.ask("akid", "AWS Access Key ID", ""),
		Secret:       w.secret("AWS Secret Access Key (hidden)"),
		SessionToken: w.secret("AWS Session Token (hidden, optional)"),
		Region:       w.ask("region", "region (optional)", ""),
	}
	if err := profiles.AddManual(a.paths.Credentials, a.paths.Config, profile, in); err != nil {
		return err
	}
	_ = profiles.SetOverride(a.paths.Overrides, profile, profiles.Override{Type: profiles.TypeManual})
	stderrf("stored profile %q (manual, key ****%s) [mode 0600]\n", profile, last4(in.AccessKeyID))
	return nil
}

func (a *app) addSSO(w *wizard, profile string) error {
	in := profiles.SSOInput{
		SessionName: w.ask("session", "sso-session name", profile),
		StartURL:    w.ask("starturl", "sso start url (https://...awsapps.com/start)", ""),
		SSORegion:   w.ask("ssoregion", "sso region", "us-east-1"),
		AccountID:   w.ask("account", "account id (optional)", ""),
		RoleName:    w.ask("role", "role name (optional)", ""),
		Region:      w.ask("region", "default region", "us-east-1"),
	}
	if err := profiles.AddSSO(a.paths.Config, profile, in); err != nil {
		return err
	}
	stderrf("stored SSO profile %q (session %q)\n", profile, in.SessionName)
	return nil
}

func (a *app) addSAML(w *wizard, profile string) error {
	in := profiles.SAMLInput{
		Account:    w.ask("account", "saml2aws account name", profile),
		URL:        w.ask("url", "IdP url", ""),
		Provider:   w.ask("provider", "provider", "GoogleApps"),
		MFA:        w.ask("mfa", "mfa", "Auto"),
		RoleARN:    w.ask("rolearn", "role_arn (optional)", ""),
		AWSProfile: w.ask("awsprofile", "aws_profile (credentials profile to write)", profile),
		Region:     w.ask("region", "region (optional)", ""),
	}
	if err := profiles.AddSAML(a.paths.Saml2aws, in); err != nil {
		return err
	}
	_ = profiles.SetOverride(a.paths.Overrides, in.AWSProfile,
		profiles.Override{Type: profiles.TypeSAML, Account: in.Account})
	stderrf("stored saml2aws account %q -> profile %q\n", in.Account, in.AWSProfile)
	return nil
}

func (a *app) addRole(w *wizard, profile string) error {
	in := profiles.RoleInput{
		RoleARN:       w.ask("rolearn", "role_arn (arn:aws:iam::...:role/...)", ""),
		SourceProfile: w.ask("source", "source_profile", ""),
		Region:        w.ask("region", "region (optional)", ""),
	}
	if err := profiles.AddRole(a.paths.Config, profile, in); err != nil {
		return err
	}
	stderrf("stored assume-role profile %q (source %q)\n", profile, in.SourceProfile)
	return nil
}

// wizard reads prompted fields from the command's stdin (or hidden TTY input
// for secrets), printing prompts to stderr so evaluable stdout stays clean.
type wizard struct {
	in *bufio.Reader
}

func newWizard(cmd *cobra.Command) *wizard {
	return &wizard{in: bufio.NewReader(cmd.InOrStdin())}
}

// ask prompts for a value, returning def when the user enters nothing.
func (w *wizard) ask(_, prompt, def string) string {
	if def != "" {
		stderrf("%s [%s]: ", prompt, def)
	} else {
		stderrf("%s: ", prompt)
	}
	line, _ := w.in.ReadString('\n')
	v := strings.TrimSpace(line)
	if v == "" {
		return def
	}
	return v
}

// secret reads a value without echo when on a TTY, else from the wizard reader.
func (w *wizard) secret(prompt string) string {
	stderrf("%s: ", prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, _ := term.ReadPassword(int(os.Stdin.Fd()))
		stderrf("\n")
		return strings.TrimSpace(string(b))
	}
	line, _ := w.in.ReadString('\n')
	return strings.TrimSpace(line)
}

func last4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}
