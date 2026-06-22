// Package auth orchestrates the login flow for a profile by dispatching to the
// right external tool based on the profile's type. It never reimplements SSO or
// SAML; it just runs `aws sso login` / `saml2aws login`.
package auth

import (
	"context"
	"fmt"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/runner"
)

// Authenticator drives logins through a CommandRunner.
type Authenticator struct {
	Runner runner.CommandRunner
}

// New returns an Authenticator.
func New(r runner.CommandRunner) *Authenticator {
	return &Authenticator{Runner: r}
}

// Plan describes the command that Login would run, without executing it.
// Useful for dry-runs, tests, and showing the user what will happen.
type Plan struct {
	// Binary is the executable required (e.g. "aws", "saml2aws"). Empty when
	// no login is needed.
	Binary string
	Args   []string
	// NoOp is true when the profile needs no interactive login (static keys).
	NoOp bool
	Note string
}

// PlanFor returns the login Plan for a profile based on its type.
func PlanFor(p profiles.Profile) Plan {
	switch p.Type {
	case profiles.TypeSSO:
		return Plan{Binary: "aws", Args: []string{"sso", "login", "--profile", p.Name}}
	case profiles.TypeSAML:
		account := p.SAMLAccount
		if account == "" {
			account = p.Name
		}
		return Plan{Binary: "saml2aws", Args: []string{"login", "-a", account, "--profile", p.Name}}
	case profiles.TypeRole:
		// Assume-role profiles do not log in directly; their source profile
		// must have valid credentials. Recursing requires the source's type,
		// which the caller resolves via ResolveRoleSource.
		return Plan{NoOp: true, Note: fmt.Sprintf("assume-role profile; ensure source profile %q is authenticated", p.SourceProfile)}
	case profiles.TypeManual:
		return Plan{NoOp: true, Note: "static keys; no login required"}
	default:
		return Plan{NoOp: true, Note: "unknown profile type; nothing to log in"}
	}
}

// Login authenticates a profile interactively (browser / prompts pass through
// to the user's terminal). For assume-role profiles it logs in the source
// profile instead, when that source is found in the provided list.
func (a *Authenticator) Login(ctx context.Context, p profiles.Profile, all []profiles.Profile) error {
	target := p
	if p.Type == profiles.TypeRole && p.SourceProfile != "" {
		if src, ok := profiles.Find(all, p.SourceProfile); ok {
			target = src
		} else {
			return fmt.Errorf("assume-role profile %q references unknown source profile %q", p.Name, p.SourceProfile)
		}
	}

	plan := PlanFor(target)
	if plan.NoOp {
		return nil
	}

	if _, err := a.Runner.LookPath(plan.Binary); err != nil {
		return fmt.Errorf("%s is not installed or not on PATH: %w", plan.Binary, err)
	}

	if err := a.Runner.RunInteractive(ctx, plan.Binary, plan.Args...); err != nil {
		return fmt.Errorf("%s login failed: %w", plan.Binary, err)
	}
	return nil
}
