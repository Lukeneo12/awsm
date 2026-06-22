// Package profiles discovers AWS profiles from the local config files and
// classifies how each one authenticates (SSO, SAML via saml2aws, assume-role,
// or static keys).
package profiles

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/ini.v1"
)

// Type is how a profile obtains credentials.
type Type string

const (
	TypeSSO     Type = "sso"     // AWS IAM Identity Center (aws sso login)
	TypeSAML    Type = "saml"    // saml2aws-managed account
	TypeRole    Type = "role"    // assume-role via source_profile
	TypeManual  Type = "manual"  // hand-managed credentials (static AKIA or pasted ASIA+token)
	TypeUnknown Type = "unknown" // present but unclassifiable
)

// ValidTypes are the user-settable profile types (excludes unknown).
var ValidTypes = []Type{TypeManual, TypeSSO, TypeSAML, TypeRole}

// IsValidType reports whether t is a user-settable type.
func IsValidType(t Type) bool {
	for _, v := range ValidTypes {
		if v == t {
			return true
		}
	}
	return false
}

// Profile is a single AWS profile and how it authenticates.
type Profile struct {
	Name   string
	Type   Type
	Region string

	// SSOSession is the [sso-session] name for SSO profiles.
	SSOSession string
	// SAMLAccount is the saml2aws account name (the -a flag) for SAML profiles.
	SAMLAccount string
	// SourceProfile is the base profile for assume-role profiles.
	SourceProfile string
	// AccessKeyIDMasked shows only the last 4 chars of the access key id for
	// keys profiles. The secret is never read into this struct.
	AccessKeyIDMasked string
}

// Paths points the loader at the config files. Empty fields fall back to the
// standard locations under the user's home directory.
type Paths struct {
	Config      string // ~/.aws/config
	Credentials string // ~/.aws/credentials
	Saml2aws    string // ~/.saml2aws
	Overrides   string // ~/.config/awsm/profiles.ini
}

// DefaultPaths returns the standard file locations for the current user.
func DefaultPaths() Paths {
	home, _ := os.UserHomeDir()
	return Paths{
		Config:      filepath.Join(home, ".aws", "config"),
		Credentials: filepath.Join(home, ".aws", "credentials"),
		Saml2aws:    filepath.Join(home, ".saml2aws"),
		Overrides:   DefaultOverridesPath(),
	}
}

// iniLoadOptions tolerates the quirks of AWS/saml2aws config files: duplicate
// keys, BOMs, and inline comments after '#'/';'.
var iniLoadOptions = ini.LoadOptions{
	AllowBooleanKeys:        true,
	AllowShadows:            true,
	SkipUnrecognizableLines: true,
}

// List discovers and classifies every profile found across the config files.
// Missing files are treated as empty (not an error); a parse error on a present
// file is returned so the caller can surface it.
func List(p Paths) ([]Profile, error) {
	cfg, err := loadINI(p.Config)
	if err != nil {
		return nil, err
	}
	creds, err := loadINI(p.Credentials)
	if err != nil {
		return nil, err
	}
	saml, err := loadINI(p.Saml2aws)
	if err != nil {
		return nil, err
	}
	overrides, err := LoadOverrides(p.Overrides)
	if err != nil {
		return nil, err
	}

	samlByProfile := samlAccounts(saml)

	// Collect the union of profile names from config and credentials.
	names := map[string]struct{}{}
	for name := range configProfiles(cfg) {
		names[name] = struct{}{}
	}
	for _, sec := range creds.SectionStrings() {
		if sec == ini.DefaultSection {
			continue
		}
		names[sec] = struct{}{}
	}
	for name := range overrides {
		names[name] = struct{}{}
	}
	// saml2aws-mapped profiles appear even before their first login writes creds.
	for awsProfile := range samlByProfile {
		names[awsProfile] = struct{}{}
	}

	var out []Profile
	for name := range names {
		out = append(out, classify(name, cfg, creds, samlByProfile, overrides))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// loadINI reads an INI file, returning an empty file if it does not exist.
func loadINI(path string) (*ini.File, error) {
	if path == "" {
		return ini.Empty(), nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ini.Empty(), nil
	}
	return ini.LoadSources(iniLoadOptions, path)
}

// configProfiles returns config section names normalized to bare profile names
// (stripping the "profile " prefix), excluding sso-session sections.
func configProfiles(cfg *ini.File) map[string]string {
	out := map[string]string{}
	for _, sec := range cfg.SectionStrings() {
		switch {
		case sec == ini.DefaultSection:
			continue
		case strings.HasPrefix(sec, "sso-session "):
			continue
		case sec == "default":
			out["default"] = sec
		case strings.HasPrefix(sec, "profile "):
			out[strings.TrimSpace(strings.TrimPrefix(sec, "profile "))] = sec
		default:
			out[sec] = sec
		}
	}
	return out
}

// samlAccounts maps the target AWS profile name -> saml2aws account name.
// saml2aws sections carry `aws_profile` (the credentials profile it writes) and
// `name` (the account passed to `saml2aws login -a <name>`).
func samlAccounts(saml *ini.File) map[string]string {
	out := map[string]string{}
	for _, sec := range saml.Sections() {
		if sec.Name() == ini.DefaultSection {
			continue
		}
		account := sec.Name()
		if n := sec.Key("name").String(); n != "" {
			account = n
		}
		awsProfile := sec.Key("aws_profile").String()
		if awsProfile == "" {
			awsProfile = account
		}
		out[awsProfile] = account
	}
	return out
}

// classify applies the precedence override > sso > saml > role > manual. It
// first enriches every metadata field from the files, then decides the Type.
func classify(name string, cfg, creds *ini.File, samlByProfile map[string]string, overrides map[string]Override) Profile {
	p := Profile{Name: name, Type: TypeUnknown}
	confSec := configSection(cfg, name)
	enrich(&p, name, confSec, creds, samlByProfile)

	// 0. User override wins outright.
	if ov, ok := overrides[name]; ok && IsValidType(ov.Type) {
		p.Type = ov.Type
		if ov.Type == TypeSAML && ov.Account != "" {
			p.SAMLAccount = ov.Account
		}
		return p
	}

	switch {
	case p.SSOSession != "" || (confSec != nil && confSec.Key("sso_start_url").String() != ""):
		p.Type = TypeSSO
	case p.SAMLAccount != "":
		p.Type = TypeSAML
	case p.SourceProfile != "":
		p.Type = TypeRole
	case p.AccessKeyIDMasked != "":
		p.Type = TypeManual
	}
	return p
}

// enrich fills the metadata fields of p from the config/credentials/saml files,
// independent of the final Type decision.
func enrich(p *Profile, name string, confSec *ini.Section, creds *ini.File, samlByProfile map[string]string) {
	if confSec != nil {
		p.Region = confSec.Key("region").String()
		p.SSOSession = confSec.Key("sso_session").String()
		if confSec.Key("role_arn").String() != "" {
			p.SourceProfile = confSec.Key("source_profile").String()
		}
	}
	if account, ok := samlByProfile[name]; ok {
		p.SAMLAccount = account
	}
	if credSec, err := creds.GetSection(name); err == nil {
		if id := credSec.Key("aws_access_key_id").String(); id != "" {
			p.AccessKeyIDMasked = maskKey(id)
			if p.Region == "" {
				p.Region = credSec.Key("region").String()
			}
		}
	}
}

// configSection returns the [profile name] (or [default]/[name]) section.
func configSection(cfg *ini.File, name string) *ini.Section {
	candidates := []string{"profile " + name, name}
	for _, c := range candidates {
		if sec, err := cfg.GetSection(c); err == nil {
			return sec
		}
	}
	return nil
}

// maskKey returns ****last4 for an access key id, never the full value.
func maskKey(id string) string {
	if len(id) <= 4 {
		return "****"
	}
	return "****" + id[len(id)-4:]
}

// Find returns the profile with the given name, or false if absent.
func Find(list []Profile, name string) (Profile, bool) {
	for _, p := range list {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}
