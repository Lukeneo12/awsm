package profiles

import (
	"path/filepath"
	"testing"
)

func TestSetAndLoadOverride_roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.ini")

	if err := SetOverride(path, "dino-dev", Override{Type: TypeManual}); err != nil {
		t.Fatalf("SetOverride error: %v", err)
	}
	if err := SetOverride(path, "dinocloud", Override{Type: TypeSAML, Account: "default"}); err != nil {
		t.Fatalf("SetOverride error: %v", err)
	}

	ov, err := LoadOverrides(path)
	if err != nil {
		t.Fatalf("LoadOverrides error: %v", err)
	}
	if ov["dino-dev"].Type != TypeManual {
		t.Errorf("dino-dev override: got %q want manual", ov["dino-dev"].Type)
	}
	if ov["dinocloud"].Type != TypeSAML || ov["dinocloud"].Account != "default" {
		t.Errorf("dinocloud override wrong: %+v", ov["dinocloud"])
	}
}

func TestSetOverride_empty_type_deletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.ini")
	if err := SetOverride(path, "p", Override{Type: TypeManual}); err != nil {
		t.Fatal(err)
	}
	if err := SetOverride(path, "p", Override{}); err != nil {
		t.Fatal(err)
	}
	ov, _ := LoadOverrides(path)
	if _, ok := ov["p"]; ok {
		t.Error("expected override to be deleted when type empty")
	}
}

func TestLoadOverrides_missing_file(t *testing.T) {
	ov, err := LoadOverrides(filepath.Join(t.TempDir(), "nope.ini"))
	if err != nil {
		t.Fatalf("expected nil err for missing file, got %v", err)
	}
	if len(ov) != 0 {
		t.Errorf("expected empty map, got %d", len(ov))
	}
}

func TestClassify_override_beats_saml(t *testing.T) {
	// Arrange: base-saml is auto-detected as SAML; override pins it to manual.
	dir := t.TempDir()
	ovPath := filepath.Join(dir, "profiles.ini")
	if err := SetOverride(ovPath, "base-saml", Override{Type: TypeManual}); err != nil {
		t.Fatal(err)
	}
	paths := Paths{
		Config:      filepath.Join("..", "..", "testdata", "config"),
		Credentials: filepath.Join("..", "..", "testdata", "credentials"),
		Saml2aws:    filepath.Join("..", "..", "testdata", "saml2aws"),
		Overrides:   ovPath,
	}

	// Act
	list, err := List(paths)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := Find(list, "base-saml")

	// Assert
	if p.Type != TypeManual {
		t.Errorf("override should force manual, got %q", p.Type)
	}
	// metadata still enriched
	if p.AccessKeyIDMasked == "" {
		t.Error("expected enriched masked key even with override")
	}
}
