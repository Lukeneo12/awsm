package cmd

import "testing"

func TestVersionString(t *testing.T) {
	t.Run("should return injected value when ldflags version is set", func(t *testing.T) {
		orig := version
		defer func() { version = orig }()
		version = "1.2.3"

		if got := versionString(); got != "1.2.3" {
			t.Errorf("versionString() = %q, want %q", got, "1.2.3")
		}
	})

	t.Run("should fall back to build info or dev when ldflags version is empty", func(t *testing.T) {
		orig := version
		defer func() { version = orig }()
		version = ""

		// Under `go test` the build info reports (devel), so the fallback
		// resolves to "dev". Either way it must never be empty.
		got := versionString()
		if got == "" {
			t.Error("versionString() returned empty string")
		}
		if got != "dev" {
			t.Errorf("versionString() = %q, want %q under go test", got, "dev")
		}
	})
}
