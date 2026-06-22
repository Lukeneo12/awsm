package status

import (
	"context"
	"testing"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/runner"
)

func TestCheck_should_report_active_when_sts_succeeds(t *testing.T) {
	// Arrange
	f := runner.NewFake()
	f.Responses["aws sts get-caller-identity --profile good"] = runner.Result{
		Stdout: []byte(`{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/x","UserId":"AIDA"}`),
	}
	c := NewChecker(f)

	// Act
	st := c.Check(context.Background(), profiles.Profile{Name: "good"})

	// Assert
	if st.State != StateActive {
		t.Fatalf("state: got %q want active", st.State)
	}
	if st.AccountID != "123456789012" {
		t.Errorf("account: got %q", st.AccountID)
	}
}

func TestCheck_should_report_expired_when_token_expired(t *testing.T) {
	f := runner.NewFake()
	f.Responses["aws sts get-caller-identity --profile sso"] = runner.Result{
		ExitCode: 255,
		Stderr:   []byte("Error loading SSO Token: Token has expired and refresh failed"),
	}
	c := NewChecker(f)

	st := c.Check(context.Background(), profiles.Profile{Name: "sso"})

	if st.State != StateExpired {
		t.Fatalf("state: got %q want expired", st.State)
	}
}

func TestCheck_should_report_invalid_on_other_failure(t *testing.T) {
	f := runner.NewFake()
	f.Responses["aws sts get-caller-identity --profile bad"] = runner.Result{
		ExitCode: 254,
		Stderr:   []byte("An error occurred (SignatureDoesNotMatch)"),
	}
	c := NewChecker(f)

	st := c.Check(context.Background(), profiles.Profile{Name: "bad"})

	if st.State != StateInvalid {
		t.Fatalf("state: got %q want invalid", st.State)
	}
}

func TestCheck_should_report_invalid_when_binary_missing(t *testing.T) {
	f := runner.NewFake()
	f.DefaultResult = runner.Result{Err: context.Canceled, ExitCode: -1}
	c := NewChecker(f)

	st := c.Check(context.Background(), profiles.Profile{Name: "any"})

	if st.State != StateInvalid {
		t.Fatalf("state: got %q want invalid", st.State)
	}
}

func TestCheckAll_should_check_every_profile(t *testing.T) {
	// Arrange
	f := runner.NewFake()
	f.Responses["aws sts get-caller-identity --profile a"] = runner.Result{
		Stdout: []byte(`{"Account":"111"}`),
	}
	f.Responses["aws sts get-caller-identity --profile b"] = runner.Result{
		ExitCode: 1, Stderr: []byte("Unable to locate credentials"),
	}
	c := NewChecker(f)
	list := []profiles.Profile{{Name: "a"}, {Name: "b"}}

	// Act
	results := c.CheckAll(context.Background(), list)

	// Assert
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results["a"].State != StateActive {
		t.Errorf("a: got %q", results["a"].State)
	}
	if results["b"].State != StateExpired {
		t.Errorf("b: got %q", results["b"].State)
	}
}
