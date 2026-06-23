# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`awsm` is a Go CLI + TUI that unifies the three ways you authenticate to AWS — IAM Identity Center (SSO), saml2aws, and static access keys — plus assume-role chains. It does **not** reimplement SSO or SAML; it orchestrates the `aws` and `saml2aws` CLIs already on the user's `PATH` and reads/writes the standard `~/.aws/*` files.

## Commands

```sh
make build    # go build -o bin/awsm .
make install  # go install .  (binary lands in $(go env GOPATH)/bin)
make test     # go test ./...
make cover    # coverage profile + total %
make vet      # go vet ./...
make fmt      # gofmt -l -w .

go test ./internal/profiles/ -run TestClassify   # single package / single test
go test ./... -run TestName -v                    # single test across packages
```

Module path is **case-sensitive**: `github.com/Lukeneo12/awsm`. Go 1.21+.

## Architecture

**Layering:** `main.go` → `cmd/` (cobra wiring) → `internal/` packages. Running `awsm` with no subcommand launches the TUI (`cmd/root.go`).

**`runner.CommandRunner` is the central seam.** Everything that shells out to `aws`/`saml2aws` goes through this interface (`internal/runner`). Tests inject `runner.Fake` (`internal/runner/fake.go`), so the entire codebase is unit-tested **without ever touching AWS or the network**. When adding any feature that runs an external command, route it through the `CommandRunner` on `app` (`cmd/root.go`) — never call `os/exec` directly. Note `Run` (captured output, non-zero exit ≠ `Err`) vs `RunInteractive` (terminal passthrough, for browser/SSO prompts).

**Profile discovery and classification** lives in `internal/profiles`. `List` reads `~/.aws/config`, `~/.aws/credentials`, `~/.saml2aws`, and the awsm override file (`~/.config/awsm/profiles.ini`), unions the profile names, and `classify` assigns a `Type` by strict precedence:

`override (set-type/add) > sso > saml > role > manual`

The **override** exists because auth method can't always be inferred from files (e.g. a stale saml2aws entry on a hand-filled profile). All AWS files are parsed with `iniLoadOptions` that tolerate duplicate keys, BOMs, and inline comments — keep using `loadINI`/those options for any new file reads. Missing files are treated as empty, not errors.

**The `switch` mechanism is the subtle part.** A child process cannot mutate its parent shell's `AWS_PROFILE`. So:
- `awsm switch <p>` prints an `export AWS_PROFILE=...` snippet to **stdout** (`internal/shell`), and a shell wrapper installed via `awsm shell-init <zsh|bash|fish>` `eval`s that output.
- **Invariant: stdout carries only the evaluable snippet; all diagnostics go to stderr** (`stderrf` in `cmd/root.go`). Breaking this corrupts the user's shell eval. Preserve it in any command whose output might be eval'd.
- The TUI can't print to the parent shell, so it writes the chosen profile to the path passed via the hidden `--switch-file` flag (set by the wrapper), which the wrapper then applies.

**Writers** (`internal/profiles/writers.go`) own all mutations of `~/.aws/credentials` (`add`, `load-credentials`, `rm`) and force mode `0600`. Secrets are read without echo and never printed — only `****last4` of the access key id is ever surfaced (`maskKey`). Keep both invariants when touching credential writes.

**TUI** is Bubble Tea (`internal/tui`): `model.go` (state/update) + `view.go` (render). Its actions (`a` add, `t` set-type, `l` load) relaunch the `awsm` wizard for the selected profile and reload.

## Conventions

- This repo follows Spec-Driven Development: specs live in `docs/specs/YYYY-MM-DD/spec-<slug>.md` and are written before non-trivial implementation. See existing specs there for the format.
- Each `internal/*` package has a colocated `_test.go`; `testdata/` holds sample `config`/`credentials`/`saml2aws` fixtures the tests load via the `Paths` struct (point `Paths` at fixtures instead of the real home dir).
