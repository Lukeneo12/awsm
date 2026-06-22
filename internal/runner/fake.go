package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Call records a single invocation made against the Fake runner.
type Call struct {
	Name string
	Args []string
}

// Fake is a deterministic CommandRunner for tests. It matches an invocation by
// the joined "name arg1 arg2 ..." string and returns the configured Result.
// Unmatched calls return a default Result with ExitCode 0 and empty output,
// unless DefaultErr is set.
type Fake struct {
	mu sync.Mutex

	// Responses maps a command key ("aws sts get-caller-identity ...") to the
	// Result to return. Matching is by prefix: the first key that the joined
	// command starts with wins, so callers can match on a stable subset.
	Responses map[string]Result
	// Missing lists executables that LookPath should report as not found.
	Missing map[string]bool
	// DefaultResult is returned when no Responses key matches.
	DefaultResult Result

	Calls            []Call
	InteractiveCalls []Call
}

// NewFake returns a Fake with initialized maps.
func NewFake() *Fake {
	return &Fake{Responses: map[string]Result{}, Missing: map[string]bool{}}
}

func (f *Fake) key(name string, args []string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

// Run records the call and returns the first matching configured Result.
func (f *Fake) Run(_ context.Context, name string, args ...string) Result {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, Call{Name: name, Args: args})
	key := f.key(name, args)
	for prefix, res := range f.Responses {
		if strings.HasPrefix(key, prefix) {
			return res
		}
	}
	return f.DefaultResult
}

// RunInteractive records the call. It returns an error only if a matching
// Response has a non-zero ExitCode or Err.
func (f *Fake) RunInteractive(_ context.Context, name string, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.InteractiveCalls = append(f.InteractiveCalls, Call{Name: name, Args: args})
	key := f.key(name, args)
	for prefix, res := range f.Responses {
		if strings.HasPrefix(key, prefix) {
			if res.Err != nil {
				return res.Err
			}
			if res.ExitCode != 0 {
				return fmt.Errorf("%s exited with code %d", name, res.ExitCode)
			}
			return nil
		}
	}
	return nil
}

// LookPath reports the binary as found unless it is listed in Missing.
func (f *Fake) LookPath(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Missing[name] {
		return "", fmt.Errorf("exec: %q: executable file not found in $PATH", name)
	}
	return "/usr/local/bin/" + name, nil
}
