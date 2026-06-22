// Package status verifies AWS sessions online by calling
// `aws sts get-caller-identity` per profile, with bounded concurrency.
package status

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/runner"
)

// State is the result of a session check.
type State string

const (
	StateActive  State = "active"  // sts returned an identity
	StateExpired State = "expired" // sts failed in a way that looks like an expired/absent session
	StateInvalid State = "invalid" // sts failed for another reason (bad keys, no network, etc.)
	StateUnknown State = "unknown" // not yet checked
)

// Status is the outcome of checking one profile.
type Status struct {
	Profile   string
	State     State
	AccountID string
	ARN       string
	// Detail carries a short human-readable reason when not active.
	Detail string
}

// callerIdentity is the JSON shape of `aws sts get-caller-identity`.
type callerIdentity struct {
	Account string `json:"Account"`
	Arn     string `json:"Arn"`
	UserID  string `json:"UserId"`
}

// Default tuning for a batch check.
const (
	defaultConcurrency = 8
	defaultTimeout     = 10 * time.Second
)

// Checker verifies sessions using a CommandRunner.
type Checker struct {
	Runner      runner.CommandRunner
	Concurrency int
	Timeout     time.Duration
}

// NewChecker returns a Checker with sane defaults.
func NewChecker(r runner.CommandRunner) *Checker {
	return &Checker{Runner: r, Concurrency: defaultConcurrency, Timeout: defaultTimeout}
}

// Check verifies a single profile by calling sts get-caller-identity.
func (c *Checker) Check(ctx context.Context, p profiles.Profile) Status {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res := c.Runner.Run(cctx, "aws", "sts", "get-caller-identity",
		"--profile", p.Name, "--output", "json")

	st := Status{Profile: p.Name}

	if res.Err != nil {
		st.State = StateInvalid
		st.Detail = res.Err.Error()
		return st
	}
	if res.ExitCode != 0 {
		st.State = classifyFailure(res.Stderr)
		st.Detail = firstLine(res.Stderr)
		return st
	}

	var id callerIdentity
	if err := json.Unmarshal(res.Stdout, &id); err != nil {
		st.State = StateInvalid
		st.Detail = "could not parse sts output"
		return st
	}
	st.State = StateActive
	st.AccountID = id.Account
	st.ARN = id.Arn
	return st
}

// CheckAll verifies every profile concurrently, bounded by Concurrency, and
// returns a map keyed by profile name.
func (c *Checker) CheckAll(ctx context.Context, list []profiles.Profile) map[string]Status {
	limit := c.Concurrency
	if limit <= 0 {
		limit = defaultConcurrency
	}

	results := make(map[string]Status, len(list))
	resCh := make(chan Status, len(list))
	sem := make(chan struct{}, limit)

	for _, p := range list {
		sem <- struct{}{}
		go func(p profiles.Profile) {
			defer func() { <-sem }()
			resCh <- c.Check(ctx, p)
		}(p)
	}

	for range list {
		st := <-resCh
		results[st.Profile] = st
	}
	return results
}

// expiredMarkers are stderr fragments that indicate an expired or absent
// session (recoverable with a login) rather than a hard credential failure.
var expiredMarkers = []string{
	"token has expired",
	"session associated with this profile has expired",
	"error loading sso token",
	"the sso session associated",
	"expiredtoken",
	"credentials have expired",
	"unable to locate credentials",
	"forbiddenexception",
}

// classifyFailure inspects stderr to distinguish an expired/absent session from
// other failures. SSO and SAML sessions surface recognizable token errors.
func classifyFailure(stderr []byte) State {
	s := strings.ToLower(string(stderr))
	for _, m := range expiredMarkers {
		if strings.Contains(s, m) {
			return StateExpired
		}
	}
	return StateInvalid
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
