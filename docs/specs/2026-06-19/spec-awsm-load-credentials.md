# Spec: awsm load-credentials (paste from clipboard)

## Context / Problem

Loading manual/temporary credentials today means the field-by-field `awsm add`
wizard. But AWS hands you a ready-made block (SSO portal "Command line or
programmatic access", `aws configure export-credentials`, etc.). Retyping each
field is friction. We want to paste that block and have awsm parse it.

## Goals

- `awsm load-credentials <profile>` (alias `load`) reads the clipboard,
  auto-detects the credentials format, and stores the credentials into the
  profile (pinning `type = manual`).
- Auto-detect: `export AWS_...`, ini `aws_access_key_id=...`, PowerShell
  `$env:AWS_...`, and CMD `set AWS_...`.
- TUI: key `l` loads the clipboard into the selected profile.
- Never print the secret/token; confirm with the masked access key id only.

## Non-goals

- Reading from stdin/file (clipboard is the chosen source; a clear error is
  shown if no clipboard tool is present).
- Storing anywhere but `~/.aws/credentials` (no vault — see prior decision).

## Acceptance criteria

- Pasting any of the four formats and running `awsm load-credentials dev`
  writes `aws_access_key_id`, `aws_secret_access_key`, and (when present)
  `aws_session_token` + region into `~/.aws/credentials` (mode 0600) and pins
  `dev` as manual.
- Missing access key or secret → clear error, nothing written.
- No clipboard tool found → error naming what to install (xclip/xsel/wl-clipboard).
- Parser and clipboard reader are unit-tested (fake runner for clipboard).

## Approach

- **`internal/creds`** (pure): `Parse(text) (Parsed, error)`. Per line: drop
  blanks/comments and `[section]` headers; strip a leading `export `, `set `,
  `$env:`/`$Env:`; split on the first `=`; trim spaces and surrounding quotes;
  match the key case-insensitively against the AWS env names and their ini
  lowercase variants. `Parsed{AccessKeyID, SecretAccessKey, SessionToken, Region}`.
- **`internal/clipboard`** `Read(r runner.CommandRunner) (string, error)`:
  pick by GOOS — darwin `pbpaste`; linux `wl-paste` → `xclip -selection
  clipboard -o` → `xsel --clipboard --output`; first one on PATH wins.
- **`cmd/loadcreds.go`**: read clipboard → `creds.Parse` → `profiles.AddManual`
  → `profiles.SetOverride(type=manual)` → confirm masked.
- **TUI**: `l` runs `awsm load-credentials <selected>` via `tea.ExecProcess`
  (reuses runSelf), then reloads.

## Risks / Rollback

- Clipboard tooling varies on Linux; handled by trying several and erroring with
  guidance. Additive feature; nothing else changes if unused.
- Secrets pass through memory only; reuse AddManual's 0600 + no-echo guarantees.
