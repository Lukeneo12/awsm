# awsm — local AWS credentials manager

[![ci](https://github.com/Lukeneo12/awsm/actions/workflows/ci.yml/badge.svg)](https://github.com/Lukeneo12/awsm/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/Lukeneo12/awsm)](https://github.com/Lukeneo12/awsm/releases)
[![license](https://img.shields.io/github/license/Lukeneo12/awsm)](LICENSE)

`awsm` unifies the three ways you authenticate to AWS — **IAM Identity Center
(SSO)**, **saml2aws**, and **static access keys** — plus **assume-role** chains,
into one CLI + TUI. It does not reimplement SSO or SAML; it orchestrates the
`aws` and `saml2aws` CLIs you already have.

## What it does

- **Switch** the active profile in your current shell (`AWS_PROFILE`).
- **Login** with the right flow per profile type (`aws sso login` / `saml2aws login`).
- **Status** — verify each session online via `aws sts get-caller-identity`.
- **Manage static keys** — add/remove profiles in `~/.aws/credentials`.

## Quick start

**Requirements:** Go 1.21+, the `aws` CLI, and (only for SAML profiles) `saml2aws` on your `PATH`.

```sh
# 1. Install the binary
go install github.com/Lukeneo12/awsm@latest

# 2. Make sure Go's bin dir is on your PATH (add this line to ~/.zshrc or ~/.bashrc)
export PATH="$(go env GOPATH)/bin:$PATH"

# 3. Install the shell wrapper so `awsm switch` can change AWS_PROFILE in your shell
#    (a child process can't mutate its parent's environment on its own)
echo 'eval "$(awsm shell-init zsh)"' >> ~/.zshrc    # bash → shell-init bash >> ~/.bashrc; fish → shell-init fish
exec $SHELL

# 4. Run it
awsm
```

> The module path is case-sensitive — install exactly `github.com/Lukeneo12/awsm@latest`.

### From source

```sh
git clone git@github.com:Lukeneo12/awsm.git
cd awsm
make install        # or: go install .
```

## Usage

```sh
awsm                 # open the interactive TUI
awsm list            # list profiles + type (offline, no network)
awsm status          # verify all sessions online
awsm status dev      # verify a single profile
awsm switch dev      # make `dev` active in this shell (needs the wrapper)
awsm login dev       # authenticate `dev` (SSO/SAML dispatched automatically)
awsm add client-x    # interactive wizard: manual | sso | saml | role
awsm add client-x --type manual   # skip the type prompt
awsm load-credentials dev         # paste-in-terminal loader (alias: load)
awsm set-type lucas-d-personal manual     # pin a profile's type (fix misclassification)
awsm set-type lucas-d-personal --clear    # back to auto-detection
awsm rm client-x     # forget a profile (credentials + config + override)
```

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

### Adding profiles

`awsm add` is a wizard that asks for the type and then the fields for that type,
writing the correct files:

- **manual** → `~/.aws/credentials` (access key + secret + optional session
  token + region), mode `0600`;
- **sso** → `~/.aws/config` (`[profile]` + `[sso-session]`);
- **saml** → `~/.saml2aws` (account block);
- **role** → `~/.aws/config` (`role_arn` + `source_profile`).

Secrets are read without echo and never printed.

### TUI keys

`↑/↓` move · `enter` login selected · `s` switch · `a` add · `l` load ·
`t` set-type · `r` refresh status · `/` filter · `q` quit

- `a` and `t` relaunch the `awsm` wizard for the selected profile, then reload.
- `l` opens a paste box: paste the credentials block, press `ctrl+d` to see a
  preview (profile, masked key, region, whether it carries a session token — the
  secret is never shown), then `y` to confirm. `esc` cancels.

## How profiles are classified

For each profile name, in order of precedence:

0. **override** — a type you pinned in `~/.config/awsm/profiles.ini` (via
   `awsm set-type` or `awsm add`) always wins.
1. **sso** — `~/.aws/config` has `sso_session` or `sso_start_url`.
2. **saml** — a `~/.saml2aws` account maps to it (`aws_profile`).
3. **role** — `~/.aws/config` has `role_arn` + `source_profile` (logging in
   authenticates the source profile).
4. **manual** — `~/.aws/credentials` has `aws_access_key_id` (static `AKIA`
   keys or pasted temporary `ASIA` + session token).

The override exists because auth method cannot always be inferred from files:
a profile may have a stale `saml2aws` entry yet be filled by hand. Pin it with
`awsm set-type <profile> manual` and it classifies correctly without touching
your AWS files.

## Security note

Static keys live in `~/.aws/credentials` in plaintext (standard AWS CLI
behavior). `awsm` mitigates this by:

- forcing `~/.aws/credentials` to mode `0600` whenever it writes;
- never printing the secret — `add` reads it without echo and confirms with only
  the last 4 characters of the access key id.

A future, more secure backend (macOS Keychain via `credential_process`) is a
documented extension, not part of this version.

## Development

```sh
make test     # run unit tests
make cover    # total coverage
make vet      # go vet
```

Every external command (`aws`, `saml2aws`) runs through the `runner.CommandRunner`
interface, so the whole codebase is unit-tested with a fake runner and never
touches AWS for real.

## License

MIT — see [LICENSE](LICENSE).
