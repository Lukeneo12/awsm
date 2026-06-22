package runner

import (
	"bytes"
	"context"
	"testing"
)

func TestExec_Run_captures_stdout(t *testing.T) {
	e := New()
	res := e.Run(context.Background(), "echo", "hello")
	if res.Err != nil {
		t.Fatalf("unexpected err: %v", res.Err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code: got %d want 0", res.ExitCode)
	}
	if got := string(bytes.TrimSpace(res.Stdout)); got != "hello" {
		t.Errorf("stdout: got %q want hello", got)
	}
}

func TestExec_Run_nonzero_exit(t *testing.T) {
	e := New()
	res := e.Run(context.Background(), "false")
	if res.Err != nil {
		t.Errorf("a ran command with nonzero exit should not set Err, got %v", res.Err)
	}
	if res.ExitCode == 0 {
		t.Errorf("expected nonzero exit code")
	}
}

func TestExec_Run_missing_binary(t *testing.T) {
	e := New()
	res := e.Run(context.Background(), "this-binary-does-not-exist-xyz")
	if res.Err == nil {
		t.Error("expected Err for missing binary")
	}
	if res.ExitCode != -1 {
		t.Errorf("exit code: got %d want -1", res.ExitCode)
	}
}

func TestExec_RunInteractive_streams(t *testing.T) {
	var out bytes.Buffer
	e := &Exec{Stdout: &out}
	if err := e.RunInteractive(context.Background(), "echo", "hi"); err != nil {
		t.Fatalf("RunInteractive error: %v", err)
	}
	if got := string(bytes.TrimSpace(out.Bytes())); got != "hi" {
		t.Errorf("interactive stdout: got %q", got)
	}
}

func TestExec_LookPath(t *testing.T) {
	e := New()
	if _, err := e.LookPath("echo"); err != nil {
		t.Errorf("expected echo on PATH: %v", err)
	}
	if _, err := e.LookPath("nope-xyz-binary"); err == nil {
		t.Error("expected error for missing binary")
	}
}

func TestFake_matches_by_prefix_and_records(t *testing.T) {
	f := NewFake()
	f.Responses["aws sts"] = Result{Stdout: []byte("ok")}

	res := f.Run(context.Background(), "aws", "sts", "get-caller-identity")
	if string(res.Stdout) != "ok" {
		t.Errorf("prefix match failed: %q", res.Stdout)
	}
	if len(f.Calls) != 1 {
		t.Errorf("expected 1 recorded call, got %d", len(f.Calls))
	}
}

func TestFake_LookPath_missing(t *testing.T) {
	f := NewFake()
	f.Missing["saml2aws"] = true
	if _, err := f.LookPath("saml2aws"); err == nil {
		t.Error("expected missing error")
	}
	if _, err := f.LookPath("aws"); err != nil {
		t.Errorf("expected aws found, got %v", err)
	}
}

func TestFake_RunInteractive_propagates_exit(t *testing.T) {
	f := NewFake()
	f.Responses["aws sso login"] = Result{ExitCode: 1}
	if err := f.RunInteractive(context.Background(), "aws", "sso", "login"); err == nil {
		t.Error("expected error from nonzero exit")
	}
}
