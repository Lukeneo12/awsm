// Package runner abstracts the execution of external commands (aws, saml2aws).
//
// Everything that shells out goes through CommandRunner so the rest of the
// codebase can be unit-tested with a fake runner and never touches AWS for real.
package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

// Result is the outcome of running an external command.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	// Err is non-nil when the command could not be started at all
	// (e.g. binary not found, context cancelled). A non-zero ExitCode from a
	// command that ran does NOT populate Err.
	Err error
}

// CommandRunner executes an external command and returns its output.
type CommandRunner interface {
	// Run captures stdout/stderr and never attaches to the terminal.
	Run(ctx context.Context, name string, args ...string) Result
	// RunInteractive wires the command to the user's terminal (stdin/stdout/
	// stderr passed through). Used for interactive logins such as
	// `aws sso login` that open a browser or prompt the user.
	RunInteractive(ctx context.Context, name string, args ...string) error
	// LookPath reports whether an executable can be found on PATH.
	LookPath(name string) (string, error)
}

// Exec is the production CommandRunner backed by os/exec.
type Exec struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// New returns an Exec wired to the process's standard streams.
func New() *Exec {
	return &Exec{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
}

// Run executes the command and captures its output buffers.
func (e *Exec) Run(ctx context.Context, name string, args ...string) Result {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res := Result{Stdout: out.Bytes(), Stderr: errBuf.Bytes()}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			// Command failed to start (not found, ctx cancelled, etc.).
			res.Err = err
			res.ExitCode = -1
		}
	}
	return res
}

// RunInteractive runs the command attached to the configured terminal streams.
func (e *Exec) RunInteractive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = e.Stdin
	cmd.Stdout = e.Stdout
	cmd.Stderr = e.Stderr
	return cmd.Run()
}

// LookPath reports the absolute path of an executable on PATH.
func (e *Exec) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}
