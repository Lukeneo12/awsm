# Spec: awsm — profile onboarding + type override

## Context / Problem

`awsm` v1 only classifies profiles by auto-detection and only adds *static keys*.
Two gaps surfaced in real use:

1. **Misclassification.** Any profile present in `~/.saml2aws` is labeled `saml`,
   even when the user manages its credentials by hand (e.g. `lucas-d-personal`: it has a
   stale saml2aws entry but is actually filled with temporary credentials pasted
   from AWS). Auth method cannot be inferred from files with certainty.
2. **Too small.** You cannot onboard a new profile (manual, SSO, SAML, or
   assume-role) or its configuration from `awsm`.

## Goals

- Let the user **pin a profile's type** via an awsm-owned override file; make
  auto-detection a fallback.
- Add an **interactive wizard** to create/configure profiles of all four types,
  writing the correct underlying files.
- A quick `set-type` command to fix classification of existing profiles without
  re-entering credentials.
- Rename the hand-managed type `keys` → `manual` (covers static `AKIA` and
  pasted temporary `ASIA`+session-token credentials).

## Non-goals

- Secure secret storage (Keychain) — still plaintext `~/.aws/credentials`,
  unchanged from v1 (documented risk).
- Editing/rotating existing SSO/SAML provider settings beyond create + region.
- Full in-TUI form editing — the TUI launches the CLI wizard as a subprocess.

## Acceptance criteria

- `~/.config/awsm/profiles.ini` with `[profile]\ntype = manual` makes `awsm list`
  show that profile as `manual`, overriding the saml2aws guess (fixes `lucas-d-personal`).
- Classification precedence: **override → sso → saml → role → manual**.
- `awsm set-type lucas-d-personal manual` writes the override and is reflected immediately.
- `awsm add <profile>` prompts for type then the fields for that type, reading
  secret/session-token without echo, and writes:
  - manual → `~/.aws/credentials` (+ region to `~/.aws/config`), mode 0600;
  - sso → `~/.aws/config` (`[profile]` + `[sso-session]`);
  - saml → `~/.saml2aws` (`[account]`), + override `type=saml, account=`;
  - role → `~/.aws/config` (`role_arn` + `source_profile`).
- Secrets are never printed; confirmation shows only last 4 of the access key id.
- In the TUI, `a` launches the add wizard and `t` launches set-type (via
  `tea.ExecProcess` running the awsm binary), refreshing on return.
- Tests cover the override store, classify-with-override, every Add writer, and
  the set-type / add (manual) commands. Total coverage ≥ 80%.

## Approach

- **New** `internal/profiles/override.go`: `Override{Type, Account}` map loaded
  from an ini file; `LoadOverrides(path)`, `SetOverride(path, profile, ov)`.
  Default path `~/.config/awsm/profiles.ini` (added to `profiles.Paths`).
- `classify()` consults the override map first.
- **New writers** in `internal/profiles`: `AddManual`, `AddSSO`, `AddSAML`,
  `AddRole` — each idempotent (overwrite section), with 0600 enforced on the
  credentials file.
- `cmd/add.go`: rewritten as a type-dispatching wizard; `cmd/settype.go`: new.
- `internal/tui`: `a` and `t` keys via `tea.ExecProcess(os.Executable(), ...)`.

## Risks / Rollback

- Unknown ini keys in `~/.saml2aws` / config: writers preserve existing keys by
  loading then mutating the target section only. Risk: reformatting comments —
  acceptable (files are tool-managed). Rollback: feature is additive; the v1
  classification path still works if the override file is absent.
- `~/.config/awsm` created with 0700; override file has no secrets.
