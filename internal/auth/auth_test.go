package auth

import (
	"context"
	"testing"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/runner"
)

func TestPlanFor(t *testing.T) {
	cases := []struct {
		name    string
		profile profiles.Profile
		binary  string
		noop    bool
		args    []string
	}{
		{"sso", profiles.Profile{Name: "dev", Type: profiles.TypeSSO}, "aws", false,
			[]string{"sso", "login", "--profile", "dev"}},
		{"saml", profiles.Profile{Name: "p", Type: profiles.TypeSAML, SAMLAccount: "acct"}, "saml2aws", false,
			[]string{"login", "-a", "acct", "--profile", "p"}},
		{"saml-fallback-account", profiles.Profile{Name: "p", Type: profiles.TypeSAML}, "saml2aws", false,
			[]string{"login", "-a", "p", "--profile", "p"}},
		{"keys", profiles.Profile{Name: "k", Type: profiles.TypeManual}, "", true, nil},
		{"role", profiles.Profile{Name: "r", Type: profiles.TypeRole, SourceProfile: "base"}, "", true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := PlanFor(tc.profile)
			if p.NoOp != tc.noop {
				t.Fatalf("noop: got %v want %v", p.NoOp, tc.noop)
			}
			if p.Binary != tc.binary {
				t.Errorf("binary: got %q want %q", p.Binary, tc.binary)
			}
			if !equalStrings(p.Args, tc.args) {
				t.Errorf("args: got %v want %v", p.Args, tc.args)
			}
		})
	}
}

func TestLogin_should_run_sso_login(t *testing.T) {
	f := runner.NewFake()
	a := New(f)
	p := profiles.Profile{Name: "dev", Type: profiles.TypeSSO}

	if err := a.Login(context.Background(), p, nil); err != nil {
		t.Fatalf("Login error: %v", err)
	}
	if len(f.InteractiveCalls) != 1 {
		t.Fatalf("expected 1 interactive call, got %d", len(f.InteractiveCalls))
	}
	call := f.InteractiveCalls[0]
	if call.Name != "aws" || call.Args[0] != "sso" {
		t.Errorf("unexpected call: %+v", call)
	}
}

func TestLogin_should_resolve_role_to_source(t *testing.T) {
	f := runner.NewFake()
	a := New(f)
	role := profiles.Profile{Name: "prod", Type: profiles.TypeRole, SourceProfile: "base"}
	source := profiles.Profile{Name: "base", Type: profiles.TypeSAML, SAMLAccount: "default"}

	if err := a.Login(context.Background(), role, []profiles.Profile{role, source}); err != nil {
		t.Fatalf("Login error: %v", err)
	}
	if len(f.InteractiveCalls) != 1 {
		t.Fatalf("expected 1 interactive call, got %d", len(f.InteractiveCalls))
	}
	if f.InteractiveCalls[0].Name != "saml2aws" {
		t.Errorf("expected saml2aws login for source, got %q", f.InteractiveCalls[0].Name)
	}
}

func TestLogin_should_error_when_role_source_missing(t *testing.T) {
	f := runner.NewFake()
	a := New(f)
	role := profiles.Profile{Name: "prod", Type: profiles.TypeRole, SourceProfile: "ghost"}

	if err := a.Login(context.Background(), role, []profiles.Profile{role}); err == nil {
		t.Error("expected error for missing source profile")
	}
}

func TestLogin_should_error_when_binary_missing(t *testing.T) {
	f := runner.NewFake()
	f.Missing["aws"] = true
	a := New(f)
	p := profiles.Profile{Name: "dev", Type: profiles.TypeSSO}

	if err := a.Login(context.Background(), p, nil); err == nil {
		t.Error("expected error when aws binary is missing")
	}
}

func TestLogin_noop_for_keys(t *testing.T) {
	f := runner.NewFake()
	a := New(f)
	if err := a.Login(context.Background(), profiles.Profile{Name: "k", Type: profiles.TypeManual}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.InteractiveCalls) != 0 {
		t.Error("keys profile should not trigger any login")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
