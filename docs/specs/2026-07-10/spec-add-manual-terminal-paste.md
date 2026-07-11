# Spec: awsm add manual — paste block in the terminal instead of field-by-field

| Field | Value |
|-------|-------|
| **Date** | 2026-07-10 |
| **Author** | Lukeneo12 |
| **Status** | Draft |
| **Type** | Feature |
| **Related PRD** | N/A |

---

## 1. Context / Problem

Creating a manual profile (`awsm add <profile> --type manual`, and the TUI `a` key) walks the user field-by-field, reading the Access Key ID, Secret and **Session Token** through hidden line prompts (`term.ReadPassword`). This is fragile precisely for the common real case: pasting a long temporary session token from the AWS SSO portal. A hidden line prompt reads only up to the first newline, so a token that carries an embedded newline (line-wrapped on copy) is silently truncated and the remainder spills into the next field (`region`) — with no echo, the user never sees it break. Reported as: "bug when creating a new profile passing the session token".

Pasting is already solved for **existing** profiles: since PR #6, `load-credentials` reads a whole AWS block from stdin until EOF (bounded at 1 MiB), parses it with the tolerant `creds.Parse`, shows a masked preview, and confirms on the controlling terminal via `internal/prompt` (`/dev/tty`, `CONIN$` on Windows). The fix is to make manual **creation** use that same flow.

This spec supersedes `docs/specs/2026-07-08/spec-awsm-add-paste-manual.md` (WIP on branch `feat/add-paste-manual`, commit `9c9ca98`): that draft read the block from the **clipboard** via `internal/clipboard`, which PR #6 deleted when it standardized on terminal paste. The TUI portion of that WIP remains a valid reference; the CLI portion is redesigned here around stdin paste.

## 2. Goals / Non-goals

### Goals
- **CLI** `awsm add <profile> --type manual` (and interactive `add` choosing manual): read an AWS credentials block pasted in the terminal (stdin until EOF), parse it with `creds.Parse`, show a masked preview (`****last4`, temporary/long-term, region), confirm on the console, then write — the same UX, seams and invariants as `load-credentials`.
- **Escape hatch**: submitting an empty paste (just EOF) falls back to the existing field-by-field wizard, covering users who have the keys as separate values to type.
- **Pipeable**: `awsm add dev --type manual < creds.txt` works; with no console available (`prompt.ErrNoTTY`) the confirmation is skipped with a notice, matching `load-credentials`.
- **TUI** `a`: pick the profile type first; for **manual**, create natively in two steps — (1) type the new profile name, (2) the existing paste textarea (`loadPaste` → `loadConfirm`) reused verbatim, storing the result with `type = manual`. Choosing **sso/saml/role** suspends into the CLI wizard preset to that type, so the TUI keeps creating every profile type.
- Extract the shared "read block → parse → preview → confirm" logic out of `cmd/loadcreds.go` so `add` and `load-credentials` cannot drift.
- Preserve both security invariants: secrets/tokens are never printed (only `****last4`), and stdout stays eval-safe (all interaction on stderr / the console).

### Non-goals
- No clipboard integration — `internal/clipboard` was removed in PR #6 and stays removed.
- No changes to SSO / SAML / role entry; those stay field-by-field in the CLI wizard.
- No changes to `profiles.AddManual` or the on-disk format.
- No change to `load-credentials` behavior (only refactoring its body into a shared helper).

## 3. Acceptance Criteria

- [ ] AC1: Given a valid AWS block piped or pasted, `awsm add dev --type manual` parses it, previews `key ****XXXX` + temporary/long-term + region on stderr, and on confirm writes access key id, secret and session token (when present) + region to `~/.aws/credentials` (mode `0600`), pins `dev` as manual — with the full session token stored intact, embedded newlines included.
- [ ] AC2: Given an empty paste (immediate EOF), the wizard falls back to the existing field-by-field entry and completes as today (existing tests stay green).
- [ ] AC3: Given a non-empty but unparseable paste, the command errors out with a hint (mirroring `load-credentials`) — it does not silently fall through to field-by-field with a half-consumed paste.
- [ ] AC4: Given input larger than 1 MiB, the command refuses to parse it (same bound and message pattern as `load-credentials`).
- [ ] AC5: Given no available console (headless/pipe), the save proceeds with a "non-interactive: saved without confirmation" notice; given an explicit `n` on the console, nothing is written.
- [ ] AC6: At no point does the secret or session token appear on stdout or stderr; stdout carries nothing (asserted the same way as `TestLoadCredsCmd_does_not_leak_secret`).
- [ ] AC7: TUI: pressing `a` shows a type menu (manual/sso/saml/role, `↑/↓`/`1-4` + enter, esc cancels); choosing manual asks for the profile name, then opens the paste textarea and confirm; on confirm the profile is created as manual and the list reloads with a success message.
- [ ] AC8: TUI: an empty or duplicate profile name at the name step is rejected with a message and the user stays on that step; esc cancels cleanly at every step.
- [ ] AC9: TUI: choosing sso/saml/role suspends the TUI into `awsm add --type <t>` and reloads the list on return.
- [ ] AC10: All new behavior is unit-tested without touching AWS, the network or the real home dir: CLI paths through `cmd.SetIn`/`SetErr` seams and the injectable `a.confirm`; TUI paths through `profiles.Paths` fixtures. Package coverage stays ≥ 80%.

## 4. Approach

### CLI (`cmd/`)

Extract the body of `load-credentials` — bounded read (`io.LimitReader`, 1 MiB + 1 sentinel byte), `creds.Parse`, masked preview, `a.confirm` with the `ErrNoTTY` auto-confirm policy — into an unexported helper (e.g. `readPastedCreds(cmd, a.confirm) (creds.Parsed, ok bool, err error)`) in `cmd/loadcreds.go`. `loadCredsCmd` keeps its exact behavior; `addManual` becomes:

1. Print `Paste the AWS credentials block, then press <EOF key> (or press <EOF key> right away to enter fields one by one):` to the command's stderr.
2. Read stdin until EOF. **Empty input → fall back to the current field-by-field wizard** (unchanged code path). Non-empty input that fails `creds.Parse` → return the parse error with the paste hint (AC3): after a consumed paste, hidden field prompts would receive nothing sensible, so failing loudly beats a confusing fallback.
3. On parse success: preview + confirm + `profiles.AddManual` + `SetOverride(manual)`, reporting `stored profile %q (manual, <kind>, key ****XXXX) [mode 0600]` via `w.errf`.

The field-by-field fallback keeps reading from the wizard's `bufio.Reader` over `cmd.InOrStdin()`; on a real terminal a Ctrl+D EOF ends the pending read but the TTY remains readable for the subsequent prompts, and in tests the seam is fed explicitly.

### TUI (`internal/tui/`)

Port the WIP from `9c9ca98`, rebased onto current `main` (which now includes the delete flow from PR #7):

- Extend the `loadStep` state machine with `loadType` (type menu) and `loadName` (name input) steps ahead of the existing `loadPaste`/`loadConfirm`; `a` enters `loadType` instead of shelling out.
- Type menu: ordered `manual, sso, saml, role` (`addableTypes()`), navigable with `↑/↓` and `1-4`, enter selects, esc cancels.
- Manual → `loadName`: plain string buffer with backspace/enter/esc handling; empty or already-existing name shows a message and stays. Enter → `loadPaste` targeting the new name, then the existing confirm → `AddManual` + manual override → reload (this last leg is the current `doLoad`, unchanged).
- sso/saml/role → `runSelf("add", "--type", string(t))` (extending `runSelf` to accept args), which suspends the TUI into the CLI wizard and reloads on return.
- View: menu and name screens mirror the existing full-screen step views; footers follow the existing help-line style.

### Key decisions
- **Terminal paste, not clipboard:** aligns with the post-#6 architecture (`internal/prompt`, `cmd.InOrStdin()` seams); no external clipboard tooling, works over SSH, and is what `load-credentials` users already learned. This is the core delta from the superseded spec.
- **Shared helper instead of duplicated flow:** `add --type manual` and `load-credentials` must present identical paste UX and invariants; one helper makes drift impossible and is directly unit-testable.
- **Empty paste = fallback, bad paste = error:** an explicit empty submission is an unambiguous "let me type them"; a failed parse after real input more likely means a copy mistake, and silently switching to hidden prompts there is exactly the confusing behavior this feature removes.
- **TUI stays native for manual, delegates the rest:** manual creation reuses the already-tested textarea/confirm machinery; sso/saml/role wizards are multi-field and interactive, so suspending into the CLI (the existing `runSelf` pattern) is simpler than rebuilding them in Bubble Tea.

### Alternatives considered
- **Clipboard as the source (the superseded spec):** rejected — `internal/clipboard` was deliberately removed in PR #6; reintroducing it would fork the paste UX and reopen the clipboard-tool dependency (pbpaste/xclip/wl-paste).
- **Silent fallback to field-by-field on any parse failure:** rejected per AC3 — hides copy mistakes behind no-echo prompts, recreating the original bug's failure mode.
- **Rebasing branch `feat/add-paste-manual` directly:** rejected — its CLI half imports a deleted package and predates the `stderrf` → `cmd.PrintErrf`/`errf` refactor; the TUI half is salvaged as reference instead.

## 5. Risks / Rollback

### Risks
| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Refactor of `loadcreds.go` regresses `load-credentials` | Low | High | Helper extraction is behavior-preserving; the existing loadcreds tests (incl. secret-leak and oversize tests) must stay green untouched |
| EOF-then-prompt interplay differs across terminals (fallback path) | Med | Med | Fallback reads via the same `bufio.Reader`/TTY path the wizard already uses; covered by seam-fed tests, and the paste path (primary) never re-prompts |
| TUI state machine growth (2 new steps) introduces interleaving bugs with the delete flow from PR #7 | Low | Med | Delete confirm is checked ahead of `loadStep` and the flows stay mutually exclusive; extend the existing interleaving tests to the new steps |
| Merge conflicts with PR #7 in `internal/tui` | High | Low | Branch from post-#7 `main`; the WIP is re-applied by hand, not rebased |

### Rollback plan
- Revert the PR merge commit (`git revert <merge-sha>`). Changes are additive to `cmd/add.go`, `cmd/loadcreds.go` (extract-only) and `internal/tui`; no data or format migrations involved.
- The superseded branch `feat/add-paste-manual` and spec remain in history for reference and can be deleted once this lands.

## 6. Open questions
- [ ] None.

---

*Spec generated with `/spec` skill. Update this file if the approach changes during implementation.*
