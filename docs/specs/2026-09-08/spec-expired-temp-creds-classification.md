# Spec: Classify stale temporary credentials as expired, not invalid

| Field | Value |
|-------|-------|
| **Date** | 2026-09-08 |
| **Author** | Lukeneo12 |
| **Status** | Draft |
| **Type** | Large Fix |
| **Related PRD** | N/A |

---

## 1. Context / Problem

The TUI status column classifies a profile's session by running `aws sts get-caller-identity` and pattern-matching its stderr (`classifyFailure` in `internal/status/status.go`). Errors matching one of the `expiredMarkers` render as `expired` (orange, recoverable with a login/re-paste); anything else renders as `invalid` (red, implies the credentials themselves are bad).

STS reports an expired **temporary** session (`ASIA…` key + `aws_session_token`) in two different ways depending on how stale it is:

- Recently expired → `ExpiredToken` → matches `expiredtoken` marker → shown as `expired` ✅
- Long expired → `InvalidClientTokenId` (STS no longer recognizes the temporary access key at all) → no marker matches → shown as `invalid` ❌

Verified on real profiles (2026-09-08): `cozify-prod` (recently expired, ASIA key) returned `ExpiredToken`; `dino-dev`, `hop-prod`, `msp-utils`, `dinocloud-apn` (long expired, all ASIA keys) returned `InvalidClientTokenId`. All of these are the same user situation — a temporary session that ran out and can be fixed by re-pasting a credentials block — but half of them are displayed as if the keys were fundamentally broken.

The distinction matters because the two states suggest different remediations: `expired` says "log in / load fresh credentials", `invalid` says "these keys are wrong, investigate".

## 2. Goals / Non-goals

### Goals

- A manual profile holding temporary credentials (session token present) whose STS check fails with `InvalidClientTokenId` is classified as `expired`.
- A profile holding long-term credentials (`AKIA…`, no session token) whose STS check fails with `InvalidClientTokenId` remains classified as `invalid` — for permanent keys that error genuinely means deactivated/deleted keys.
- Classification stays purely local and offline-safe: the decision uses only the STS stderr plus credential metadata already on disk; no extra AWS calls are added.
- No secret material is read into new code paths beyond what is needed to detect "has a session token / key id prefix"; nothing new is ever printed.

### Non-goals

- Not adding new states or changing the meaning of `active` / `expired` / `invalid` / `unknown`.
- Not parsing credential expiry timestamps (e.g. `x_security_token_expires`) or predicting expiry without calling STS.
- Not changing how SSO / SAML / role failures are classified — their token errors already match the existing markers.
- Not reworking the marker heuristic itself (it stays a stderr substring match).
- Not touching the TUI rendering or badge styles.

## 3. Acceptance Criteria

- [ ] AC1: Given a profile whose credentials file entry contains an `aws_session_token`, when `aws sts get-caller-identity` exits non-zero with stderr containing `InvalidClientTokenId`, then `Checker.Check` returns `StateExpired`.
- [ ] AC2: Given a profile whose credentials file entry has an access key but **no** `aws_session_token`, when the check fails with stderr containing `InvalidClientTokenId`, then `Checker.Check` returns `StateInvalid`.
- [ ] AC3: Given a profile with no credentials file entry at all (e.g. pure SSO/SAML profile), when the check fails with stderr containing `InvalidClientTokenId`, then `Checker.Check` returns `StateInvalid` (unchanged behavior).
- [ ] AC4: All existing classifications are unchanged: every current `expiredMarkers` case still returns `StateExpired`, successful checks still return `StateActive`, runner errors still return `StateInvalid`.
- [ ] AC5: New unit tests cover AC1–AC3 using `runner.Fake` and fixture credentials files via the `Paths` struct — no network, no real `~/.aws` access.
- [ ] AC6: No new code path prints, logs, or returns secret material; at most the presence of a session token and the key id prefix are inspected in memory.
- [ ] AC7: `go test ./...` and `go vet ./...` pass; package coverage for `internal/status` does not drop below its current level.

## 4. Approach

Teach the classifier about the credential *shape* of the profile being checked, so ambiguous STS errors can be resolved by context.

1. **Expose a "has temporary credentials" signal.** Add a small helper (in `internal/profiles` or `internal/creds`, wherever the credentials file is already parsed with `loadINI`) that reports, for a profile name, whether its `~/.aws/credentials` section contains an `aws_session_token`. It reads via the existing `Paths` struct and tolerant INI options; missing file or missing section simply means "no".
2. **Thread that signal into `Checker.Check`.** `Checker` gains a `HasSessionToken func(profile string) bool` field, and `NewChecker(runner, paths)` takes the same `profiles.Paths` the profile list was discovered from, wiring the default lookup against it — the caller's paths, not `DefaultPaths()` resolved internally, so discovery and classification always read the same files (raised in review of PR #13). `classifyFailure` becomes context-aware: signature `classifyFailure(stderr []byte, tempCreds bool)`.
3. **Add a conditional marker set.** A new list `tempCredsExpiredMarkers` containing `invalidclienttokenid` (and the phrase `security token included in the request is invalid`) is consulted **only when** `hasSessionToken` is true. The unconditional `expiredMarkers` list stays as-is.
4. **Tests.** Table-driven cases in `internal/status/status_test.go` with `runner.Fake` stderr fixtures for `ExpiredToken`, `InvalidClientTokenId` × {session token present, absent, no credentials entry}, plus a fixture credentials file under `testdata/`.

### Key decisions

- **Decision 1: gate on session-token presence, not on `ASIA` key prefix.** The session token is the definitive on-disk marker of temporary credentials; the `ASIA`/`AKIA` prefix is an AWS convention that could drift and adds nothing when the token is already visible. Tradeoff: hand-crafted entries with a stale leftover session token next to permanent keys would be classified as expired — acceptable, since that entry is malformed anyway and "reload credentials" is still the right remediation.
- **Decision 2: keep the heuristic in `classifyFailure`, extended with context, rather than moving classification into the TUI or profiles layer.** `internal/status` already owns the state taxonomy; spreading it out would duplicate the marker lists.
- **Decision 3: read the session-token presence from disk at check time instead of caching it on `profiles.Profile`.** Credentials can change between profile discovery and a status refresh (that is the whole point of `load-credentials`); reading at check time avoids stale classification after a reload. Cost is one small local file read per check, which is negligible next to the STS network call.

### Alternatives considered

- **Option A: add `invalidclienttokenid` to the global `expiredMarkers`.** Rejected: for long-term `AKIA` keys this error genuinely means bad/deactivated keys, and showing them as `expired` would tell the user to "just re-login" when the keys need replacing.
- **Option B: parse credential expiry timestamps locally and skip STS.** Rejected: not all temporary blocks carry an expiry key, clocks drift, and it changes the architecture from "verify online" to "predict offline" — out of scope.
- **Option C: introduce a distinct fourth state (e.g. `stale`).** Rejected: it adds UI and mental overhead without changing the remediation, which is identical to `expired`.

## 5. Risks / Rollback

### Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| A permanent-keys profile with a leftover stale `aws_session_token` line shows `expired` instead of `invalid` | Low | Low | Remediation shown ("reload credentials") still fixes it; documented in Decision 1 |
| Reading the credentials file per check adds I/O in `CheckAll`'s concurrent goroutines | Low | Low | Read-only access to a small local file; no locking needed, writers already own mutations |
| Marker string drift if AWS CLI changes its error wording | Low | Med | Same exposure as the existing marker list; substring match on the error code (`invalidclienttokenid`) is the most stable fragment |

### Rollback plan

Single revert of the implementation commit/PR restores the previous behavior — the change is additive (new conditional marker list + helper), touches no persisted state, no file formats, and no CLI flags. No data migration involved.

## 6. Open questions

- [ ] None — approach was validated against live profiles before writing this spec.

---

*Spec generated with `/spec` skill. Update this file if the approach changes during implementation.*
