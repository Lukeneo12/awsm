package cmd

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	t.Run("should return injected value when ldflags version is set", func(t *testing.T) {
		orig := version
		defer func() { version = orig }()
		version = "1.2.3"

		if got := versionString(); got != "1.2.3" {
			t.Errorf("versionString() = %q, want %q", got, "1.2.3")
		}
	})

	t.Run("should strip v prefix from injected value", func(t *testing.T) {
		orig := version
		defer func() { version = orig }()
		version = "v1.2.3"

		if got := versionString(); got != "1.2.3" {
			t.Errorf("versionString() = %q, want %q", got, "1.2.3")
		}
	})

	t.Run("should never be empty when ldflags version is empty", func(t *testing.T) {
		orig := version
		defer func() { version = orig }()
		version = ""

		// The fallback resolves via build info (or "dev" under go test,
		// where it reports (devel)); the invariant is a non-empty,
		// unprefixed value either way.
		got := versionString()
		if got == "" {
			t.Error("versionString() returned empty string")
		}
		if strings.HasPrefix(got, "v") {
			t.Errorf("versionString() = %q, want no v prefix", got)
		}
	})
}
