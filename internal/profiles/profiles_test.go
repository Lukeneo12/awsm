package profiles

import (
	"path/filepath"
	"testing"
)

func testPaths() Paths {
	return Paths{
		Config:      filepath.Join("..", "..", "testdata", "config"),
		Credentials: filepath.Join("..", "..", "testdata", "credentials"),
		Saml2aws:    filepath.Join("..", "..", "testdata", "saml2aws"),
	}
}

func TestList_should_classify_all_profiles_when_files_present(t *testing.T) {
	// Arrange
	paths := testPaths()

	// Act
	list, err := List(paths)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	// Assert
	byName := map[string]Profile{}
	for _, p := range list {
		byName[p.Name] = p
	}
	if len(list) != 5 {
		t.Fatalf("expected 5 profiles, got %d: %+v", len(list), list)
	}

	cases := []struct {
		name        string
		wantType    Type
		wantRegion  string
		wantExtra   string // sso-session / saml account / source / masked key
		extraGetter func(Profile) string
	}{
		{"sso-dev", TypeSSO, "us-east-1", "my-sso", func(p Profile) string { return p.SSOSession }},
		{"legacy-sso", TypeSSO, "eu-west-1", "", func(p Profile) string { return p.SSOSession }},
		{"role-prod", TypeRole, "us-west-2", "base-saml", func(p Profile) string { return p.SourceProfile }},
		{"base-saml", TypeSAML, "", "default", func(p Profile) string { return p.SAMLAccount }},
		{"static-keys", TypeManual, "sa-east-1", "****1234", func(p Profile) string { return p.AccessKeyIDMasked }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := byName[tc.name]
			if !ok {
				t.Fatalf("profile %q not found", tc.name)
			}
			if p.Type != tc.wantType {
				t.Errorf("type: got %q want %q", p.Type, tc.wantType)
			}
			if p.Region != tc.wantRegion {
				t.Errorf("region: got %q want %q", p.Region, tc.wantRegion)
			}
			if got := tc.extraGetter(p); got != tc.wantExtra {
				t.Errorf("extra: got %q want %q", got, tc.wantExtra)
			}
		})
	}
}

func TestList_should_return_empty_when_files_missing(t *testing.T) {
	// Arrange
	paths := Paths{
		Config:      "does-not-exist-config",
		Credentials: "does-not-exist-credentials",
		Saml2aws:    "does-not-exist-saml",
	}

	// Act
	list, err := List(paths)

	// Assert
	if err != nil {
		t.Fatalf("expected no error for missing files, got %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(list))
	}
}

func TestFind(t *testing.T) {
	list := []Profile{{Name: "a"}, {Name: "b"}}
	if _, ok := Find(list, "b"); !ok {
		t.Error("expected to find b")
	}
	if _, ok := Find(list, "z"); ok {
		t.Error("did not expect to find z")
	}
}

func TestMaskKey(t *testing.T) {
	cases := map[string]string{
		"AKIAEXAMPLE000001234": "****1234",
		"abc":                  "****",
		"":                     "****",
	}
	for in, want := range cases {
		if got := maskKey(in); got != want {
			t.Errorf("maskKey(%q)=%q want %q", in, got, want)
		}
	}
}
