package cmd

import "runtime/debug"

// version is injected at release time via ldflags
// (-X github.com/Lukeneo12/awsm/cmd.version=...). Empty for local builds.
var version string

// versionString resolves the version to report: the ldflags-injected value
// wins; otherwise fall back to the module version recorded by the Go
// toolchain (set for `go install module@version` builds), or "dev".
func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
