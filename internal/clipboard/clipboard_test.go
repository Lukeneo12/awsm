package clipboard

import (
	"testing"

	"github.com/Lukeneo12/awsm/internal/runner"
)

func TestRead_darwin_uses_pbpaste(t *testing.T) {
	f := runner.NewFake()
	f.Responses["pbpaste"] = runner.Result{Stdout: []byte("clipboard contents")}

	got, err := read(f, "darwin")
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if got != "clipboard contents" {
		t.Errorf("got %q", got)
	}
}

func TestRead_linux_falls_back_to_xclip(t *testing.T) {
	f := runner.NewFake()
	f.Missing["wl-paste"] = true // not installed -> skip
	f.Responses["xclip -selection clipboard -o"] = runner.Result{Stdout: []byte("via xclip")}

	got, err := read(f, "linux")
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if got != "via xclip" {
		t.Errorf("got %q", got)
	}
}

func TestRead_no_tool_found(t *testing.T) {
	f := runner.NewFake()
	f.Missing["wl-paste"] = true
	f.Missing["xclip"] = true
	f.Missing["xsel"] = true

	if _, err := read(f, "linux"); err == nil {
		t.Error("expected error when no clipboard tool is available")
	}
}

func TestRead_unsupported_os(t *testing.T) {
	if _, err := read(runner.NewFake(), "plan9"); err == nil {
		t.Error("expected error for unsupported OS")
	}
}

func TestRead_propagates_tool_failure(t *testing.T) {
	f := runner.NewFake()
	f.Responses["pbpaste"] = runner.Result{ExitCode: 1, Stderr: []byte("boom")}
	if _, err := read(f, "darwin"); err == nil {
		t.Error("expected error when the tool exits nonzero")
	}
}
