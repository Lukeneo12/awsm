package profiles

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

// credFileMode is the only acceptable permission for the credentials file.
const credFileMode os.FileMode = 0o600

// ManualInput is the data for a hand-managed credentials profile.
type ManualInput struct {
	AccessKeyID  string
	Secret       string
	SessionToken string // optional; present for temporary (ASIA) credentials
	Region       string
}

// SSOInput is the data for an IAM Identity Center profile.
type SSOInput struct {
	SessionName string // [sso-session NAME]
	StartURL    string
	SSORegion   string
	AccountID   string
	RoleName    string
	Region      string
}

// SAMLInput is the data for a saml2aws account.
type SAMLInput struct {
	Account    string // saml2aws account name (the -a flag)
	URL        string
	Provider   string
	MFA        string
	RoleARN    string
	AWSProfile string // credentials profile saml2aws writes to
	Region     string
}

// RoleInput is the data for an assume-role profile.
type RoleInput struct {
	RoleARN       string
	SourceProfile string
	Region        string
}

// AddManual writes a hand-managed credentials profile to the credentials file
// (and its region to config), enforcing 0600 and never logging the secret.
func AddManual(credentialsPath, configPath, profile string, in ManualInput) error {
	if profile == "" || in.AccessKeyID == "" || in.Secret == "" {
		return fmt.Errorf("profile, access key id and secret are all required")
	}
	f, err := loadOrCreateSecure(credentialsPath)
	if err != nil {
		return err
	}
	sec := f.Section(profile)
	sec.Key("aws_access_key_id").SetValue(in.AccessKeyID)
	sec.Key("aws_secret_access_key").SetValue(in.Secret)
	if in.SessionToken != "" {
		sec.Key("aws_session_token").SetValue(in.SessionToken)
	} else {
		sec.DeleteKey("aws_session_token")
	}
	if in.Region != "" {
		sec.Key("region").SetValue(in.Region)
	}
	if err := saveSecure(f, credentialsPath); err != nil {
		return err
	}
	if in.Region != "" && configPath != "" {
		return setConfigKeys(configPath, profile, map[string]string{"region": in.Region})
	}
	return nil
}

// AddSSO writes an SSO profile and its sso-session block to the config file.
func AddSSO(configPath, profile string, in SSOInput) error {
	if profile == "" || in.StartURL == "" || in.SessionName == "" {
		return fmt.Errorf("profile, sso start url and session name are required")
	}
	f, err := loadOrCreatePlain(configPath)
	if err != nil {
		return err
	}
	prof := f.Section("profile " + profile)
	prof.Key("sso_session").SetValue(in.SessionName)
	setIfNotEmpty(prof, "sso_account_id", in.AccountID)
	setIfNotEmpty(prof, "sso_role_name", in.RoleName)
	setIfNotEmpty(prof, "region", in.Region)

	sess := f.Section("sso-session " + in.SessionName)
	sess.Key("sso_start_url").SetValue(in.StartURL)
	setIfNotEmpty(sess, "sso_region", in.SSORegion)
	sess.Key("sso_registration_scopes").SetValue("sso:account:access")
	return f.SaveTo(configPath)
}

// AddRole writes an assume-role profile to the config file.
func AddRole(configPath, profile string, in RoleInput) error {
	if profile == "" || in.RoleARN == "" || in.SourceProfile == "" {
		return fmt.Errorf("profile, role_arn and source_profile are required")
	}
	return setConfigKeys(configPath, profile, map[string]string{
		"role_arn":       in.RoleARN,
		"source_profile": in.SourceProfile,
		"region":         in.Region,
	})
}

// AddSAML writes a saml2aws account to the saml2aws config file.
func AddSAML(samlPath string, in SAMLInput) error {
	if in.Account == "" || in.URL == "" {
		return fmt.Errorf("account and url are required")
	}
	if in.AWSProfile == "" {
		in.AWSProfile = in.Account
	}
	if in.Provider == "" {
		in.Provider = "GoogleApps"
	}
	if in.MFA == "" {
		in.MFA = "Auto"
	}
	f, err := loadOrCreatePlain(samlPath)
	if err != nil {
		return err
	}
	sec := f.Section(in.Account)
	sec.Key("name").SetValue(in.Account)
	sec.Key("url").SetValue(in.URL)
	sec.Key("provider").SetValue(in.Provider)
	sec.Key("mfa").SetValue(in.MFA)
	sec.Key("aws_profile").SetValue(in.AWSProfile)
	setIfNotEmpty(sec, "role_arn", in.RoleARN)
	setIfNotEmpty(sec, "region", in.Region)
	return f.SaveTo(samlPath)
}

// RemoveProfile deletes a profile section from the credentials file. It is a
// no-op (nil) if the file or section does not exist.
func RemoveProfile(credentialsPath, profile string) error {
	if _, err := os.Stat(credentialsPath); os.IsNotExist(err) {
		return nil
	}
	f, err := ini.LoadSources(iniLoadOptions, credentialsPath)
	if err != nil {
		return err
	}
	if _, err := f.GetSection(profile); err != nil {
		return nil
	}
	f.DeleteSection(profile)
	return saveSecure(f, credentialsPath)
}

// RemoveConfigProfile deletes a profile's section ([profile X] or [X]) from the
// config file. No-op (nil) if the file or section does not exist.
func RemoveConfigProfile(configPath, profile string) error {
	if configPath == "" {
		return nil
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}
	f, err := ini.LoadSources(iniLoadOptions, configPath)
	if err != nil {
		return err
	}
	removed := false
	for _, name := range []string{"profile " + profile, profile} {
		if _, err := f.GetSection(name); err == nil {
			f.DeleteSection(name)
			removed = true
		}
	}
	if !removed {
		return nil
	}
	return f.SaveTo(configPath)
}

// CheckPermissions reports whether the credentials file is more permissive than
// 0600. Returns (tooOpen=true, current mode) when group/other bits are set.
func CheckPermissions(credentialsPath string) (tooOpen bool, mode os.FileMode, err error) {
	info, err := os.Stat(credentialsPath)
	if err != nil {
		return false, 0, err
	}
	mode = info.Mode().Perm()
	return mode&0o077 != 0, mode, nil
}

// --- helpers ---

// setConfigKeys merges keys into a [profile name] section of the config file.
func setConfigKeys(configPath, profile string, keys map[string]string) error {
	f, err := loadOrCreatePlain(configPath)
	if err != nil {
		return err
	}
	sec := f.Section("profile " + profile)
	for k, v := range keys {
		setIfNotEmpty(sec, k, v)
	}
	return f.SaveTo(configPath)
}

func setIfNotEmpty(sec *ini.Section, key, val string) {
	if val != "" {
		sec.Key(key).SetValue(val)
	}
}

// loadOrCreatePlain loads an ini file, creating its directory and an empty file
// (mode 0644) if absent. Used for non-secret files (config, saml2aws).
func loadOrCreatePlain(path string) (*ini.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ini.Empty(), nil
	}
	return ini.LoadSources(iniLoadOptions, path)
}

// loadOrCreateSecure loads the credentials file, creating it with 0600 if absent.
func loadOrCreateSecure(path string) (*ini.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, nil, credFileMode); err != nil {
			return nil, err
		}
		return ini.Empty(), nil
	}
	return ini.LoadSources(iniLoadOptions, path)
}

// saveSecure writes the credentials file and forces 0600.
func saveSecure(f *ini.File, path string) error {
	if err := f.SaveTo(path); err != nil {
		return err
	}
	return os.Chmod(path, credFileMode)
}
