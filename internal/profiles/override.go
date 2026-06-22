package profiles

import (
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

// Override is an awsm-owned, user-declared classification for a profile. It
// takes precedence over auto-detection so ambiguous cases (e.g. a profile that
// has a stale saml2aws entry but is filled by hand) classify correctly.
type Override struct {
	Type    Type
	Account string // saml2aws account name, when Type == TypeSAML
}

// overridesMode keeps the awsm config directory tight.
const overridesDirMode os.FileMode = 0o700

// DefaultOverridesPath returns ~/.config/awsm/profiles.ini.
func DefaultOverridesPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "awsm", "profiles.ini")
}

// LoadOverrides reads the override file, returning an empty map if it is absent.
func LoadOverrides(path string) (map[string]Override, error) {
	out := map[string]Override{}
	if path == "" {
		return out, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return out, nil
	}
	f, err := ini.LoadSources(iniLoadOptions, path)
	if err != nil {
		return nil, err
	}
	for _, sec := range f.Sections() {
		if sec.Name() == ini.DefaultSection {
			continue
		}
		t := Type(sec.Key("type").String())
		if t == "" {
			continue
		}
		out[sec.Name()] = Override{Type: t, Account: sec.Key("account").String()}
	}
	return out, nil
}

// SetOverride writes (or updates) a profile's override and returns nil. Passing
// an empty Type deletes the override for that profile.
func SetOverride(path, profile string, ov Override) error {
	if err := os.MkdirAll(filepath.Dir(path), overridesDirMode); err != nil {
		return err
	}
	var f *ini.File
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f = ini.Empty()
	} else {
		var lerr error
		if f, lerr = ini.LoadSources(iniLoadOptions, path); lerr != nil {
			return lerr
		}
	}

	if ov.Type == "" {
		f.DeleteSection(profile)
		return f.SaveTo(path)
	}

	sec := f.Section(profile)
	sec.Key("type").SetValue(string(ov.Type))
	if ov.Account != "" {
		sec.Key("account").SetValue(ov.Account)
	} else {
		sec.DeleteKey("account")
	}
	return f.SaveTo(path)
}
