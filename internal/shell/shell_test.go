package shell

import (
	"strings"
	"testing"

	"github.com/Lukeneo12/awsm/internal/profiles"
)

func TestExportSnippet_posix(t *testing.T) {
	p := profiles.Profile{Name: "dev", Region: "us-east-1"}
	got := ExportSnippet(p, "bash")
	if !strings.Contains(got, "export AWS_PROFILE=dev") {
		t.Errorf("missing AWS_PROFILE export: %q", got)
	}
	if !strings.Contains(got, "export AWS_REGION=us-east-1") {
		t.Errorf("missing AWS_REGION export: %q", got)
	}
}

func TestExportSnippet_omits_region_when_empty(t *testing.T) {
	got := ExportSnippet(profiles.Profile{Name: "dev"}, "zsh")
	if strings.Contains(got, "AWS_REGION") {
		t.Errorf("should not emit AWS_REGION when region empty: %q", got)
	}
}

func TestExportSnippet_fish(t *testing.T) {
	got := ExportSnippet(profiles.Profile{Name: "dev", Region: "eu-west-1"}, "fish")
	if !strings.Contains(got, "set -gx AWS_PROFILE dev") {
		t.Errorf("fish export wrong: %q", got)
	}
}

func TestWrapper_supported_shells(t *testing.T) {
	for _, sh := range []string{"zsh", "bash", "fish"} {
		w, err := Wrapper(sh)
		if err != nil {
			t.Fatalf("Wrapper(%q) error: %v", sh, err)
		}
		if !strings.Contains(w, "awsm") {
			t.Errorf("wrapper for %q does not mention awsm", sh)
		}
		if !strings.Contains(w, "switch") {
			t.Errorf("wrapper for %q does not handle switch", sh)
		}
	}
}

func TestWrapper_unsupported_shell(t *testing.T) {
	if _, err := Wrapper("powershell"); err == nil {
		t.Error("expected error for unsupported shell")
	}
}
