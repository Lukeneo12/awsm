# load-credentials Terminal Paste + Confirm — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `awsm load-credentials` read the AWS credentials block from stdin (paste in terminal, end with EOF), show a masked preview, and save only after the user confirms on the console — replacing the clipboard source.

**Architecture:** A new `internal/prompt` package asks a yes/no question on the controlling terminal (`/dev/tty` on Unix, `CONIN$`/`CONOUT$` on Windows) behind a build-tagged seam, keeping the decision logic unit-testable. `cmd/loadcreds.go` reads the block from `cmd.InOrStdin()`, parses it with the existing `creds.Parse`, prints a masked preview to stderr, and confirms via an injectable `app.confirm` field. The now-unused `internal/clipboard` package is deleted.

**Tech Stack:** Go (1.21+), cobra, `gopkg.in/ini.v1` (existing). No new dependencies — `prompt` uses only the standard library.

## Global Constraints

- Go 1.21+. No new module dependencies.
- stdout stays eval-safe: every prompt, preview, notice, and the success line goes to **stderr** (via `stderrf`).
- The secret/session token is **never printed**; only `****last4` of the access key id is shown.
- Credentials are written by the existing `profiles.AddManual` (mode `0600`) — do not change it.
- Confirmation reads from the console, not stdin. Console open is platform-specific behind build tags; no new deps.
- Paste-prompt EOF key is platform-correct: `Ctrl+D` (Unix) / `Ctrl+Z` then Enter (Windows), via `prompt.EOFKey`.
- Non-interactive (no console): auto-confirm and print `non-interactive: saved without confirmation` to stderr.

---

## File Structure

- `internal/prompt/prompt.go` (create) — `Confirm`, the testable `confirm` core, `ErrNoTTY`.
- `internal/prompt/console_unix.go` (create, `//go:build !windows`) — `openConsole` via `/dev/tty`; `EOFKey = "Ctrl+D"`.
- `internal/prompt/console_windows.go` (create, `//go:build windows`) — `openConsole` via `CONIN$`/`CONOUT$`; `EOFKey = "Ctrl+Z then Enter"`.
- `internal/prompt/prompt_test.go` (create) — table test for the `confirm` core.
- `cmd/root.go` (modify) — add `confirm` field to `app`, default it to `prompt.Confirm` in `newApp`.
- `cmd/loadcreds.go` (modify) — read from stdin, preview, confirm, save.
- `cmd/cmd_test.go` (modify) — replace the two clipboard tests with stdin+confirm tests.
- `internal/clipboard/clipboard.go` + `internal/clipboard/clipboard_test.go` (delete).
- `README.md` (modify) — rewrite the load-credentials section; drop clipboard-tool requirement.

---

### Task 1: `internal/prompt` package

**Files:**
- Create: `internal/prompt/prompt.go`
- Create: `internal/prompt/console_unix.go`
- Create: `internal/prompt/console_windows.go`
- Test: `internal/prompt/prompt_test.go`

**Interfaces:**
- Consumes: nothing (stdlib only).
- Produces:
  - `func Confirm(question string) (bool, error)` — asks on the console; returns `ErrNoTTY` if no console.
  - `var ErrNoTTY error`
  - `const EOFKey string` (platform-specific)
  - package-internal `func confirm(r io.Reader, w io.Writer, question string) (bool, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/prompt/prompt_test.go`:

```go
package prompt

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfirm_core(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"lower y", "y\n", true},
		{"word yes", "yes\n", true},
		{"upper Y", "Y\n", true},
		{"upper YES", "YES\n", true},
		{"y padded with spaces", "  y  \n", true},
		{"empty is no", "\n", false},
		{"n is no", "n\n", false},
		{"word no", "no\n", false},
		{"garbage is no", "maybe\n", false},
		{"eof without newline", "y", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := confirm(strings.NewReader(tc.input), &out, "Save?")
			if err != nil {
				t.Fatalf("confirm error: %v", err)
			}
			if got != tc.want {
				t.Errorf("confirm(%q) = %v, want %v", tc.input, got, tc.want)
			}
			if !strings.Contains(out.String(), "Save? [y/N]: ") {
				t.Errorf("prompt not written to w: %q", out.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/prompt/ -run TestConfirm_core -v`
Expected: FAIL — build error, `undefined: confirm`.

- [ ] **Step 3: Write the package core**

Create `internal/prompt/prompt.go`:

```go
// Package prompt asks the user a yes/no question on the console, independently
// of stdin. The block being confirmed is typically read from stdin until EOF,
// so the confirmation must come from a separate channel — the controlling
// terminal (/dev/tty on Unix, CONIN$/CONOUT$ on Windows). All console access
// lives behind a build-tagged seam (openConsole) so the decision logic stays
// unit-testable.
package prompt

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// ErrNoTTY is returned by Confirm when no console can be opened (e.g. a headless
// CI run). Callers decide the non-interactive policy.
var ErrNoTTY = errors.New("no console available")

// Confirm asks question on the console and reports whether the user agreed.
// It returns ErrNoTTY when the console cannot be opened.
func Confirm(question string) (bool, error) {
	r, w, closeFn, err := openConsole()
	if err != nil {
		return false, ErrNoTTY
	}
	defer closeFn()
	return confirm(r, w, question)
}

// confirm writes the prompt to w and reads one line from r, returning true only
// for an explicit yes (y/yes, case-insensitive). An empty line (just Enter) or
// anything else is false. It holds the testable logic, free of console I/O.
func confirm(r io.Reader, w io.Writer, question string) (bool, error) {
	if _, err := io.WriteString(w, question+" [y/N]: "); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
```

- [ ] **Step 4: Write the platform seams**

Create `internal/prompt/console_unix.go`:

```go
//go:build !windows

package prompt

import (
	"io"
	"os"
)

// EOFKey is the keystroke that signals end-of-input when pasting into stdin.
const EOFKey = "Ctrl+D"

// openConsole opens the controlling terminal for interactive prompts.
func openConsole() (r io.Reader, w io.Writer, closeFn func(), err error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	return f, f, func() { _ = f.Close() }, nil
}
```

Create `internal/prompt/console_windows.go`:

```go
//go:build windows

package prompt

import (
	"io"
	"os"
)

// EOFKey is the keystroke that signals end-of-input when pasting into stdin.
const EOFKey = "Ctrl+Z then Enter"

// openConsole opens the Windows console (CONIN$/CONOUT$), which reach the real
// console even when stdin/stdout are redirected.
func openConsole() (r io.Reader, w io.Writer, closeFn func(), err error) {
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		_ = in.Close()
		return nil, nil, nil, err
	}
	return in, out, func() { _ = in.Close(); _ = out.Close() }, nil
}
```

- [ ] **Step 5: Run tests + vet to verify they pass**

Run: `go test ./internal/prompt/ -v && go vet ./internal/prompt/`
Expected: PASS (all `TestConfirm_core` subtests), no vet output.

- [ ] **Step 6: Commit**

```bash
git add internal/prompt/
git commit -m "feat: add prompt package for console y/n confirmation

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Swap clipboard for stdin + confirm in the CLI

**Files:**
- Modify: `cmd/root.go`
- Modify: `cmd/loadcreds.go`
- Modify: `cmd/cmd_test.go`
- Delete: `internal/clipboard/clipboard.go`, `internal/clipboard/clipboard_test.go`

**Interfaces:**
- Consumes: `prompt.Confirm`, `prompt.ErrNoTTY`, `prompt.EOFKey` (Task 1); existing `creds.Parse`, `profiles.AddManual`, `profiles.SetOverride`, `last4`, `stderrf`.
- Produces: `app.confirm func(question string) (bool, error)` field (tests inject a stub).

- [ ] **Step 1: Write the failing tests**

In `cmd/cmd_test.go`, add `"github.com/Lukeneo12/awsm/internal/prompt"` to the imports, then **replace** `TestLoadCredsCmd_from_clipboard` and `TestLoadCredsCmd_bad_clipboard_content` with:

```go
func loadApp(t *testing.T, confirm func(string) (bool, error)) *app {
	t.Helper()
	dir := t.TempDir()
	return &app{
		paths: profiles.Paths{
			Credentials: filepath.Join(dir, "credentials"),
			Config:      filepath.Join(dir, "config"),
			Overrides:   filepath.Join(dir, "profiles.ini"),
		},
		runner:  runner.NewFake(),
		confirm: confirm,
	}
}

func TestLoadCredsCmd_confirm_saves(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return true, nil })
	c := a.loadCredsCmd()
	c.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=\"ASIAEXAMPLE9999\"\n" +
			"export AWS_SECRET_ACCESS_KEY=\"thesecret\"\n" +
			"export AWS_SESSION_TOKEN=\"thetoken\"\n" +
			"export AWS_DEFAULT_REGION=\"us-east-1\"\n"))
	c.SetArgs([]string{"dino-dev"})
	if err := c.Execute(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	p, ok := profiles.Find(list, "dino-dev")
	if !ok || p.Type != profiles.TypeManual {
		t.Fatalf("expected dino-dev manual, got %+v (ok=%v)", p, ok)
	}
	if p.AccessKeyIDMasked != "****9999" {
		t.Errorf("masked key: got %q", p.AccessKeyIDMasked)
	}
}

func TestLoadCredsCmd_decline_writes_nothing(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return false, nil })
	c := a.loadCredsCmd()
	c.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=ASIA9999\nexport AWS_SECRET_ACCESS_KEY=s\n"))
	c.SetArgs([]string{"dino-dev"})
	if err := c.Execute(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "dino-dev"); ok {
		t.Error("declining should write nothing")
	}
}

func TestLoadCredsCmd_non_interactive_auto_confirms(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return false, prompt.ErrNoTTY })
	c := a.loadCredsCmd()
	c.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=AKIA1234\nexport AWS_SECRET_ACCESS_KEY=s\n"))
	c.SetArgs([]string{"ci-prof"})
	if err := c.Execute(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	if p, ok := profiles.Find(list, "ci-prof"); !ok || p.Type != profiles.TypeManual {
		t.Fatalf("non-interactive run should save, got ok=%v", ok)
	}
}

func TestLoadCredsCmd_bad_content_errors(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return true, nil })
	c := a.loadCredsCmd()
	c.SetIn(strings.NewReader("not credentials at all"))
	c.SetArgs([]string{"x"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when input has no credentials")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run TestLoadCredsCmd -v`
Expected: FAIL — `a.confirm` undefined (and old behavior reads clipboard).

- [ ] **Step 3: Add the `confirm` seam to `app`**

In `cmd/root.go`, add the `prompt` import and extend `app` + `newApp`:

```go
import (
	"fmt"
	"os"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/prompt"
	"github.com/Lukeneo12/awsm/internal/runner"
	"github.com/Lukeneo12/awsm/internal/tui"
	"github.com/spf13/cobra"
)

// app holds shared dependencies for the commands.
type app struct {
	paths   profiles.Paths
	runner  runner.CommandRunner
	confirm func(question string) (bool, error)
}

func newApp() *app {
	return &app{
		paths:   profiles.DefaultPaths(),
		runner:  runner.New(),
		confirm: prompt.Confirm,
	}
}
```

- [ ] **Step 4: Rewrite `cmd/loadcreds.go`**

Replace the whole file with:

```go
package cmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/Lukeneo12/awsm/internal/creds"
	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/prompt"
	"github.com/spf13/cobra"
)

func (a *app) loadCredsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "load-credentials <profile>",
		Aliases: []string{"load"},
		Short:   "Load manual credentials for a profile by pasting them in the terminal",
		Long: "load-credentials reads an AWS credentials block from stdin, auto-detects the\n" +
			"format (export, ini, PowerShell or cmd), shows a masked preview and stores it\n" +
			"into the profile in ~/.aws/credentials (mode 0600), pinning its type as manual.\n" +
			"Paste the block AWS gives you, then press " + prompt.EOFKey + ". You can also pipe\n" +
			"it in: awsm load <profile> < creds.txt. The secret is never printed.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := args[0]

			stderrf("Paste the AWS credentials block, then press %s:\n", prompt.EOFKey)
			raw, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("reading pasted credentials: %w", err)
			}
			parsed, err := creds.Parse(string(raw))
			if err != nil {
				return fmt.Errorf("%w (paste an AWS credentials block, then press %s)", err, prompt.EOFKey)
			}

			kind := "long-term"
			if parsed.SessionToken != "" {
				kind = "temporary"
			}
			region := parsed.Region
			if region == "" {
				region = "(none)"
			}
			stderrf("About to load into %q:\n", profile)
			stderrf("  key:    ****%s\n", last4(parsed.AccessKeyID))
			stderrf("  region: %s\n", region)
			stderrf("  type:   %s\n", kind)

			ok, err := a.confirm("Save?")
			switch {
			case errors.Is(err, prompt.ErrNoTTY):
				stderrf("non-interactive: saved without confirmation\n")
				ok = true
			case err != nil:
				return err
			}
			if !ok {
				stderrf("aborted, nothing written\n")
				return nil
			}

			in := profiles.ManualInput{
				AccessKeyID:  parsed.AccessKeyID,
				Secret:       parsed.SecretAccessKey,
				SessionToken: parsed.SessionToken,
				Region:       parsed.Region,
			}
			if err := profiles.AddManual(a.paths.Credentials, a.paths.Config, profile, in); err != nil {
				return err
			}
			_ = profiles.SetOverride(a.paths.Overrides, profile, profiles.Override{Type: profiles.TypeManual})

			stderrf("loaded %s credentials into %q (key ****%s) [mode 0600]\n",
				kind, profile, last4(parsed.AccessKeyID))
			return nil
		},
	}
}
```

- [ ] **Step 5: Delete the clipboard package**

```bash
git rm internal/clipboard/clipboard.go internal/clipboard/clipboard_test.go
```

- [ ] **Step 6: Verify no references to clipboard remain**

Run: `grep -rn "internal/clipboard\|clipboard\." --include="*.go" .`
Expected: no output (exit 1).

- [ ] **Step 7: Run the full test suite + vet + Windows cross-compile**

Run: `go build ./... && go test ./... && go vet ./... && GOOS=windows go build ./...`
Expected: build succeeds; all packages PASS; no vet output; Windows cross-compile
succeeds (catches typos in `console_windows.go`, which the local platform skips).

- [ ] **Step 8: Commit**

```bash
git add cmd/root.go cmd/loadcreds.go cmd/cmd_test.go internal/clipboard
git commit -m "feat: load-credentials reads from terminal paste + confirms

Reads the credentials block from stdin (Ctrl+D / Ctrl+Z), shows a masked
preview, and saves only after console confirmation. Non-interactive runs
auto-confirm. Removes the now-unused internal/clipboard package.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Update the README

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: nothing (docs only).
- Produces: nothing.

- [ ] **Step 1: Find every clipboard mention**

Run: `grep -n "clipboard\|pbpaste\|wl-paste\|xclip\|xsel\|load-credentials\|paste-from" README.md`
Expected: the Usage-line comment, the "Loading credentials from the clipboard" section. (The TUI `l` description stays — the TUI keeps its paste box.)

- [ ] **Step 2: Update the Usage-line comment**

Change:
```
awsm load-credentials dev         # paste-from-clipboard loader (alias: load)
```
to:
```
awsm load-credentials dev         # paste-in-terminal loader (alias: load)
```

- [ ] **Step 3: Rewrite the load-credentials section**

Replace the `### Loading credentials from the clipboard` section (heading + body) with:

```markdown
### Loading credentials by pasting them

When AWS hands you a credentials block (the SSO portal's "Command line or
programmatic access", `aws configure export-credentials`, etc.), run:

```sh
awsm load-credentials dev
```

`awsm` prompts you to paste the block, ending with `Ctrl+D` (`Ctrl+Z` then Enter
on Windows). It auto-detects the format — `export AWS_...`, an ini
`aws_access_key_id=...` block, PowerShell `$env:AWS_...`, or cmd `set AWS_...` —
shows a masked preview (profile, masked key, region, whether it carries a session
token), and on `y` stores the access key, secret, optional session token and
region into the profile (mode `0600`, pinned as `manual`). The secret is never
printed. You can also pipe the block in:

```sh
awsm load-credentials dev < creds.txt
```
```

- [ ] **Step 4: Verify the requirement line no longer mentions clipboard tools**

Run: `grep -n "pbpaste\|wl-paste\|xclip\|xsel" README.md`
Expected: no output (exit 1).

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: README reflects terminal-paste load-credentials

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Notes for the implementer

- Do not modify `internal/creds` (the parser) or `profiles.AddManual` — they are reused unchanged.
- The TUI (`internal/tui`) keeps its own paste textarea and is out of scope; do not touch it.
- `last4` already exists in `cmd/add.go` and is reused by `loadcreds.go` — do not redefine it.
- `golang.org/x/term` is already a dependency (used by `add.go`); the `prompt` package intentionally avoids it and uses only stdlib, so `go.mod`/`go.sum` must not change.
