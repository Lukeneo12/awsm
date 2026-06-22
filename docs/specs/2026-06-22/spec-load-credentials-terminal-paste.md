# Spec: awsm load-credentials (paste in terminal + confirm)

## Context / Problem

`awsm load-credentials <profile>` currently reads the AWS credentials block from
the system clipboard and writes it straight to `~/.aws/credentials`, with no
confirmation. Two problems:

- **No clipboard, no load.** On hosts without a clipboard tool (headless boxes,
  remote shells over SSH, restricted environments) the command is unusable —
  there is no way to feed it the block.
- **No confirmation.** It writes immediately, so a stale or wrong clipboard
  silently overwrites a profile's credentials.

We want the CLI to prompt the user to paste the block directly in the terminal,
show a masked preview, and only save after the user confirms. This also brings
the CLI in line with the TUI, which already does paste → preview → `y/n` confirm.

## Goals

- `awsm load-credentials <profile>` (alias `load`) reads the credentials block
  from **stdin** (paste in terminal, end with EOF — `Ctrl+D` on Unix,
  `Ctrl+Z` then Enter on Windows) instead of the clipboard.
- Show a **masked preview** before saving — profile, masked access key id,
  region, and whether the block carries a session token (long-term vs
  temporary) — and require confirmation.
- Confirmation is read from the **console** (`/dev/tty` on Unix, `CONIN$` on
  Windows), not stdin, so it works even when the block is piped in
  (`awsm load dev < file`). Cross-platform — no new dependencies.
- Piping support falls out for free: `awsm load dev < creds.txt`.
- The secret/session token is **never printed**; preview shows only `****last4`
  of the access key id.
- Remove the now-unused `internal/clipboard` package and update the README.

## Non-goals

- Keeping a clipboard input mode in the CLI (decision: replace, not add a flag).
  The TUI keeps its own paste textarea and is unaffected.
- A `--yes`/`--force` flag. Non-interactive runs auto-confirm (see Approach);
  no flag is introduced.
- Any change to the parser (`internal/creds`), to `profiles.AddManual`, or to
  the on-disk format/permissions.

## Acceptance criteria

- Running `awsm load-credentials dev` prints a paste prompt to **stderr** (with
  the platform-correct EOF key — `Ctrl+D` on Unix, `Ctrl+Z` then Enter on
  Windows), reads the pasted block from stdin until EOF, and parses it with the
  existing `creds.Parse` (all four formats still supported).
- After parsing, a masked preview is shown (profile, `****last4`, region,
  long-term vs temporary). The secret and session token never appear in output.
- **Interactive** (controlling terminal available, including when the block is
  piped via `< file`): the user is prompted `Save? [y/N]:` on the console.
  - `y`/`yes` (case-insensitive) → `profiles.AddManual` + `SetOverride(manual)`,
    then the existing success line to stderr.
  - anything else → abort, **nothing written**.
- **Non-interactive** (no controlling terminal, e.g. CI): no prompt; the
  credentials are saved and a `non-interactive: saved without confirmation`
  notice is printed to stderr.
- Missing access key or secret → clear error, nothing written (unchanged).
- stdout carries no diagnostics — prompt, preview, notices, and the success line
  all go to stderr (preserves the eval-safe stdout invariant).
- `internal/clipboard` is deleted; `grep` finds no remaining references; the
  build and `go test ./...` pass.

## Approach

- **`internal/prompt`** (new, small, in the style of `internal/runner`):
  - `Confirm(question string) (bool, error)` — opens the console, writes
    `question + " [y/N]: "`, reads one line, returns true only for `y`/`yes`
    (case-insensitive). Enter (empty line) → false. Console open is the only
    untestable bit and lives behind a build-tagged seam.
  - A package-internal `confirm(r io.Reader, w io.Writer, question string)`
    holds the testable logic (prompt + read + interpret); the exported `Confirm`
    wires it to the console.
  - **Console seam** via build tags (no new dependencies):
    - `console_unix.go` (`//go:build !windows`) opens `/dev/tty` (read+write).
    - `console_windows.go` (`//go:build windows`) opens `CONIN$` (read) and
      `CONOUT$` (write) with `os.OpenFile`, which reach the real console even
      when stdin/stdout are redirected.
    - Either returns a sentinel (`ErrNoTTY`) when the console cannot be opened
      (truly headless) so the caller can apply the non-interactive policy.
- **`cmd/loadcreds.go`**:
  - Read the block from `a.stdin` (new field, defaults `os.Stdin`) via
    `io.ReadAll`, after printing the paste prompt to stderr.
  - `creds.Parse` (unchanged) → on error, the existing friendly message.
  - Build the masked preview from the parsed fields and print to stderr.
  - Confirm via `a.confirm` (new field, defaults to a fn calling
    `prompt.Confirm`). On `ErrNoTTY`, auto-confirm and print the non-interactive
    notice. On `false`, abort without writing.
  - On confirm: `profiles.AddManual` + `SetOverride(type=manual)` + success line
    (all unchanged).
- **`cmd/root.go`**: extend `app` with `stdin io.Reader` and
  `confirm func(question string) (bool, error)`; `newApp` defaults them to
  `os.Stdin` and the `prompt.Confirm` wrapper. This mirrors how `runner` is
  injected for testability.
- **Delete** `internal/clipboard/clipboard.go` + `clipboard_test.go`.
- **README**: rewrite the "Loading credentials from the clipboard" section to
  describe paste-in-terminal + `Ctrl+D` + confirm, mention piping, and drop the
  `pbpaste`/`wl-paste`/`xclip`/`xsel` requirement.

## Risks / Rollback

- **Behavior change**: existing muscle memory / scripts that relied on the
  clipboard source break. Mitigated by piping support (`< file`) covering the
  scripted case and a clear new prompt for the interactive case. Documented in
  the README and PR.
- **Console portability**: handled per-platform behind a build-tagged seam
  (`/dev/tty` on Unix, `CONIN$`/`CONOUT$` on Windows), so the interactive path
  works on all three OSes. The paste-prompt EOF key is platform-specific
  (`Ctrl+D` / `Ctrl+Z`). If no console can be opened at all (truly headless),
  the non-interactive path (auto-confirm) applies, which is safe (still writes
  0600, still masks output).
- **Auto-confirm in non-interactive mode** could write unintended creds in a
  pipeline. Acceptable: the user explicitly invoked `load <profile>` and piped a
  block; the notice makes it visible. Rollback is a single revert — the parser,
  writers, and TUI are untouched, so nothing else regresses.
