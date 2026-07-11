# Spec: Delete profiles from the TUI

| Field | Value |
|-------|-------|
| **Date** | 2026-07-10 |
| **Author** | Lukeneo12 |
| **Status** | Draft |
| **Type** | Feature |
| **Related PRD** | N/A |

---

## 1. Context / Problem

`awsm` can already forget a profile from the CLI: `awsm rm <profile>` (aliases `remove`, `forget`) deletes the profile's section from `~/.aws/credentials` and `~/.aws/config` and clears its awsm type override. The TUI, however, has no equivalent: its actions today are `a` (add), `t` (set-type), `l` (load credentials), `s` (switch), `enter` (login), `r` (refresh). A user managing profiles interactively must quit the TUI and run the CLI command to remove one — or may not discover that removal is possible at all.

Desired state: the profile list in the TUI supports deleting the selected profile with an explicit confirmation step, using exactly the same removal semantics as `awsm rm`.

## 2. Goals / Non-goals

### Goals
- Delete the selected profile from within the TUI via a dedicated key (`d`).
- Require an explicit y/n confirmation before deleting (destructive action), showing the profile name and its detected type.
- Reuse the existing removal logic (`profiles.RemoveProfile`, `profiles.RemoveConfigProfile`, `profiles.SetOverride` with an empty override) so TUI and CLI behavior never diverge.
- Reload the profile list and re-run status checks after a successful delete, and surface a result message (success or error) in the TUI message line.
- Advertise the new key in the TUI help footer.

### Non-goals
- No changes to the `awsm rm` CLI command (no confirmation prompt, no `--yes` flag; CLI stays as-is).
- No bulk/multi-select deletion.
- No "undo" — recovery is manual (see Rollback).
- No deletion of `~/.saml2aws` entries (matches current `rm` behavior, which does not touch that file).

## 3. Acceptance Criteria

- [ ] AC1: Given the TUI profile list with a profile selected, when the user presses `d`, then a confirmation prompt is shown displaying the profile name and type, and the list keybindings are suspended.
- [ ] AC2: Given the confirmation prompt, when the user presses `y` (or `Y`), then the profile's section is removed from the credentials file, its section (`[profile X]` or `[X]`) is removed from the config file, its awsm override is cleared, the profile list reloads without the profile, and a success message ("✓ deleted <name>") is shown.
- [ ] AC3: Given the confirmation prompt, when the user presses `n`, `esc`, or any other key, then nothing is deleted, the TUI returns to the list view, and a "cancelled" message is shown.
- [ ] AC4: Given an empty (or fully filtered-out) profile list, when the user presses `d`, then nothing happens (no prompt, no crash).
- [ ] AC5: Given a removal that fails (e.g. unwritable credentials file), when the user confirms, then the error is shown in the message line, the TUI keeps running, and the profile list is reloaded from disk so it never shows state a partial failure already changed.
- [ ] AC6: The list-view help footer includes the `d` action (e.g. `d delete`).
- [ ] AC7: The credentials file keeps mode `0600` after deletion (existing writers invariant holds).
- [ ] AC8: All new behavior is covered by unit tests in `internal/tui/model_test.go` (confirm/cancel/empty-list/error paths) without touching the real home directory (tests point `profiles.Paths` at fixtures).

## 4. Approach

All changes live in `internal/tui` (`model.go`, `view.go`) plus tests. No new packages, no CLI changes.

**State.** Add a `deleteTarget string` field to `model` (empty = no pending delete). This mirrors how the load flow tracks its target, but a single field is enough — the delete flow has one step. The load flow's `loadStep` state machine is left untouched.

**Update.** In `handleKey` (list view), add `case "d"`: if `m.selected()` returns a profile, set `deleteTarget` to its name and clear `m.message`. In the main key dispatch (before list handling, alongside the `loadConfirm` branch), when `deleteTarget != ""` route keys to a new `handleDeleteConfirmKey`:
- `y`/`Y` → call `profiles.RemoveProfile(m.paths.Credentials, name)`, `profiles.RemoveConfigProfile(m.paths.Config, name)`, and `profiles.SetOverride(m.paths.Overrides, name, profiles.Override{})`, stopping at the first error. Set `m.message` to the error or to `"✓ deleted <name>"`, clear `deleteTarget`, and return `m.reload()` on **every** path — success or failure. Reloading after a failed delete matters because an earlier writer call may already have mutated disk (e.g. credentials removed, config write failed); the list must keep mirroring what is actually on disk instead of showing the pre-delete state until a manual refresh. *(Updated during implementation: the first version reloaded only on success; QA flagged the stale-list gap and the approach was corrected.)*
- anything else → clear `deleteTarget`, set a "deletion cancelled" message.

Deletion runs in-process against `m.paths` (like `doLoad` already does for `AddManual`) rather than shelling out to `awsm rm` via `runSelf`: the operation is non-interactive, needs no terminal handoff, and in-process calls keep it testable with fixture paths.

**View.** In `View()`, when `deleteTarget != ""` render a confirmation screen (same pattern as `confirmView` for loads): title `Delete profile → <name>`, the profile's type, a warning that this removes it from credentials and config and clears the override, and the `y confirm · n / esc cancel` help line. Add `d delete` to the list footer.

**Messages/i18n note:** existing TUI copy mixes Spanish and English; new user-facing strings follow the nearest existing pattern (footer in English like the current one, confirm prompt mirroring the load confirm style).

### Key decisions
- **In-process removal instead of `runSelf("rm")`:** reuses the same `internal/profiles` writers the CLI uses, avoids a pointless terminal suspend/resume, and keeps the path injectable for tests. Trade-off: the "what was removed" reporting logic in `cmd/rm.go` is not shared, but it is three writer calls — divergence risk is minimal and both sides are covered by tests.
- **Dedicated `deleteTarget` field instead of extending the `loadStep` state machine:** the load state machine models a multi-step flow; delete is a single confirm step. Overloading `loadStep` would couple unrelated flows and complicate `endLoad()` semantics.
- **Confirmation is mandatory, default-deny:** any key other than `y`/`Y` cancels. Matches the destructive nature of the action and the existing load-confirm convention.

### Alternatives considered
- **Option A: shell out to `awsm rm` via `runSelf`** — rejected: `rm` is non-interactive, so `tea.ExecProcess` adds screen flicker and makes unit testing require a fake executable; in-process writer calls are strictly simpler.
- **Option B: add a confirmation/`--yes` flag to CLI `rm` and share a single "confirm+remove" helper** — rejected as out of scope: user decided TUI-only; changing CLI `rm` behavior could break scripts that rely on it being non-interactive.

## 5. Risks / Rollback

### Risks
| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| User deletes the wrong profile (destructive, no undo) | Med | High | Mandatory y/n confirm showing name + type; default-deny on any other key |
| Removal partially succeeds (credentials removed, config write fails) | Low | Med | Surface the error immediately and reload the list so it reflects disk; re-running delete is idempotent (writers treat missing sections/files as no-ops) |
| Key collision or regression in existing keybindings | Low | Med | `d` is currently unbound in list view; tests cover existing bindings still working |

### Rollback plan
- Code: revert the feature commit(s) on the branch / revert the PR merge commit (`git revert <merge-sha>`). The change is additive and isolated to `internal/tui`.
- Data: a deleted profile is not recoverable by awsm; the user re-adds it via `awsm add` / TUI `a` (documented behavior, same as CLI `rm` today).

## 6. Open questions
- [ ] None.

---

*Spec generated with `/spec` skill. Update this file if the approach changes during implementation.*
