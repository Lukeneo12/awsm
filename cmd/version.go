package cmd

import (
	"runtime/debug"
	"strings"
)

// version is injected at release time via ldflags
// (-X github.com/Lukeneo12/awsm/cmd.version=...). Empty for local builds.
var version string

// versionString resolves the version to report: the ldflags-injected value
// wins; otherwise fall back to the module version recorded by the Go
// toolchain (set for `go install module@version` builds), or "dev".
// The "v" tag prefix is stripped so both paths report the same format
// (GoReleaser's {{.Version}} already comes without it, build info with it).
func versionString() string {
	if version != "" {
		return strings.TrimPrefix(version, "v")
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "dev"
}
