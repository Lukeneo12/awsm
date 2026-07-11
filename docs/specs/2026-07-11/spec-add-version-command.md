# Spec: Add --version flag to the awsm CLI

| Field | Value |
|-------|-------|
| **Date** | 2026-07-11 |
| **Author** | Lukeneo12 |
| **Status** | Draft |
| **Type** | Feature |
| **Related PRD** | N/A |

---

## 1. Context / Problem

`awsm` currently has no way to report its own version: `awsm --version` and `awsm version` both fail with "unknown flag/command". Users cannot tell which release they are running, which makes bug reports and upgrade checks ("am I on v0.2.0?") guesswork.

Releases are already tagged (`v0.2.0` is current) and built with GoReleaser, which can inject the version at build time. Binaries are also commonly built via `go install` / `make install`, where no ldflags are passed, so the feature needs a sensible fallback for that path.

## 2. Goals / Non-goals

### Goals
- `awsm --version` prints the version and exits 0.
- Release binaries (GoReleaser) report the tag version (e.g. `awsm version 0.2.0`).
- `go install github.com/Lukeneo12/awsm@vX.Y.Z` builds report the module version from Go build info, without ldflags.
- Local `make install` / `go build` builds report a recognizable dev value instead of an empty string.

### Non-goals
- A `version` subcommand (the `--version` flag is enough; cobra prints a clear error suggesting flags for unknown commands).
- Self-update or update-check functionality.
- Embedding commit hash / build date (can be added later if needed).

## 3. Acceptance Criteria

- [ ] AC1: `awsm --version` exits 0 and prints exactly one line matching `awsm version <value>` to stdout.
- [ ] AC2: A GoReleaser build for tag `vX.Y.Z` prints `awsm version X.Y.Z` (verified via `goreleaser build --snapshot` printing the snapshot version).
- [ ] AC3: A plain `go build` binary prints a non-empty fallback (module version from `debug.ReadBuildInfo()`, or `dev` when build info reports `(devel)`).
- [ ] AC4: All other commands and the TUI launch path behave exactly as before (`go test ./...` passes; no change to stdout of `awsm switch`).
- [ ] AC5: Unit test covers the version-resolution helper: ldflags value wins over build info; build-info fallback used when ldflags value is empty.

## 4. Approach

Use cobra's built-in version support: setting `Version` on the root command makes cobra register `--version` and print `awsm version <v>` via its default template. No new subcommand code.

Changes:

1. **`cmd/version.go` (new)** — `var version string` (ldflags injection point) and `func versionString() string`: returns `version` if non-empty; otherwise reads `debug.ReadBuildInfo().Main.Version`, mapping `(devel)`/empty to `dev`.
2. **`cmd/root.go`** — set `Version: versionString()` on the root command.
3. **`.goreleaser.yaml`** — extend ldflags: `-s -w -X github.com/Lukeneo12/awsm/cmd.version={{.Version}}`.
4. **`cmd/version_test.go` (new)** — table test for `versionString()` covering the injected and fallback paths.

### Key decisions
- **Inject into `cmd` package, not `main`:** the version is consumed by cobra wiring in `cmd`; injecting there avoids threading a parameter through `cmd.Execute()`. Tradeoff: the ldflags path is longer than `main.version`, but it is set once in `.goreleaser.yaml`.
- **`debug.ReadBuildInfo()` fallback:** gives correct versions for `go install module@version` builds with zero build tooling. Local builds show `dev`, which is honest and unambiguous.
- **Flag only, no subcommand:** matches the request, smallest surface; cobra gives the flag for free once `Version` is set.

### Alternatives considered
- **Custom `version` subcommand:** rejected — duplicates what cobra's `Version` field provides and grows the command tree for no gain.
- **Version from git at runtime:** rejected — installed binaries don't live in the repo; unreliable.

## 5. Risks / Rollback

### Risks
| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| ldflags path typo → release binaries print `dev` | Low | Low | AC2 verifies via `goreleaser build --snapshot` before merging |
| Version output interferes with shell-wrapper eval | Low | Med | `--version` is a normal passthrough arg in the wrapper (`command awsm "$@"`); no `switch` stdout involved. Covered by AC4 |

### Rollback plan
Single small PR; revert the commit. No data, config, or file-format changes.

## 6. Open questions

- None.

---

*Spec generated with `/spec` skill. Update this file if the approach changes during implementation.*
