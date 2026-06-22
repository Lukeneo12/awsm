package profiles

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/ini.v1"
)

func TestAddManual_should_write_creds_with_strict_perms(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	configPath := filepath.Join(dir, "config")

	err := AddManual(credPath, configPath, "newprof", ManualInput{
		AccessKeyID:  "ASIATESTKEY000099",
		Secret:       "supersecretvalue",
		SessionToken: "tok123",
		Region:       "us-east-2",
	})
	if err != nil {
		t.Fatalf("AddManual error: %v", err)
	}

	info, _ := os.Stat(credPath)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perms: got %o want 600", info.Mode().Perm())
	}

	f, _ := ini.Load(credPath)
	sec := f.Section("newprof")
	if sec.Key("aws_access_key_id").String() != "ASIATESTKEY000099" {
		t.Error("access key id not stored")
	}
	if sec.Key("aws_secret_access_key").String() != "supersecretvalue" {
		t.Error("secret not stored")
	}
	if sec.Key("aws_session_token").String() != "tok123" {
		t.Error("session token not stored")
	}

	cf, _ := ini.Load(configPath)
	if cf.Section("profile newprof").Key("region").String() != "us-east-2" {
		t.Error("region not written to config")
	}

	list, _ := List(Paths{Credentials: credPath, Config: configPath})
	if p, _ := Find(list, "newprof"); p.Type != TypeManual {
		t.Errorf("expected manual, got %q", p.Type)
	}
}

func TestAddManual_should_reject_incomplete(t *testing.T) {
	dir := t.TempDir()
	if err := AddManual(filepath.Join(dir, "creds"), "", "p", ManualInput{AccessKeyID: "", Secret: "s"}); err == nil {
		t.Error("expected error when access key id empty")
	}
}

func TestAddSSO_writes_profile_and_session(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	err := AddSSO(configPath, "mysso", SSOInput{
		SessionName: "corp", StartURL: "https://corp.awsapps.com/start",
		SSORegion: "us-east-1", AccountID: "123456789012", RoleName: "Admin", Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("AddSSO error: %v", err)
	}

	f, _ := ini.Load(configPath)
	if f.Section("profile mysso").Key("sso_session").String() != "corp" {
		t.Error("sso_session not set")
	}
	if f.Section("sso-session corp").Key("sso_start_url").String() != "https://corp.awsapps.com/start" {
		t.Error("sso_start_url not set")
	}

	list, _ := List(Paths{Config: configPath})
	if p, _ := Find(list, "mysso"); p.Type != TypeSSO {
		t.Errorf("expected sso, got %q", p.Type)
	}
}

func TestAddRole_writes_role_profile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	if err := AddRole(configPath, "prod", RoleInput{
		RoleARN: "arn:aws:iam::222222222222:role/X", SourceProfile: "base", Region: "us-west-2",
	}); err != nil {
		t.Fatalf("AddRole error: %v", err)
	}

	list, _ := List(Paths{Config: configPath})
	p, _ := Find(list, "prod")
	if p.Type != TypeRole || p.SourceProfile != "base" {
		t.Errorf("expected role with source base, got %+v", p)
	}
}

func TestAddSAML_writes_account(t *testing.T) {
	dir := t.TempDir()
	samlPath := filepath.Join(dir, "saml2aws")
	credPath := filepath.Join(dir, "credentials")

	err := AddSAML(samlPath, SAMLInput{
		Account: "acme", URL: "https://accounts.google.com/o/saml2", RoleARN: "arn:aws:iam::1:role/r",
		AWSProfile: "acme-prof",
	})
	if err != nil {
		t.Fatalf("AddSAML error: %v", err)
	}

	f, _ := ini.Load(samlPath)
	sec := f.Section("acme")
	if sec.Key("aws_profile").String() != "acme-prof" {
		t.Error("aws_profile not set")
	}
	if sec.Key("provider").String() != "GoogleApps" {
		t.Error("expected default provider GoogleApps")
	}

	list, _ := List(Paths{Saml2aws: samlPath, Credentials: credPath})
	if p, _ := Find(list, "acme-prof"); p.Type != TypeSAML || p.SAMLAccount != "acme" {
		t.Errorf("expected saml account acme, got %+v", p)
	}
}

func TestRemoveProfile_should_delete_section(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	if err := AddManual(credPath, "", "victim", ManualInput{AccessKeyID: "ASIA1", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveProfile(credPath, "victim"); err != nil {
		t.Fatalf("RemoveProfile error: %v", err)
	}
	f, _ := ini.Load(credPath)
	if _, err := f.GetSection("victim"); err == nil {
		t.Error("section victim should have been deleted")
	}
}

func TestRemoveConfigProfile_deletes_section(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := AddRole(configPath, "prod", RoleInput{RoleARN: "arn:x", SourceProfile: "base"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveConfigProfile(configPath, "prod"); err != nil {
		t.Fatalf("RemoveConfigProfile error: %v", err)
	}
	f, _ := ini.Load(configPath)
	if _, err := f.GetSection("profile prod"); err == nil {
		t.Error("expected [profile prod] to be deleted")
	}
}

func TestRemoveConfigProfile_noop_when_missing(t *testing.T) {
	if err := RemoveConfigProfile(filepath.Join(t.TempDir(), "nope"), "x"); err != nil {
		t.Errorf("expected nil for missing file, got %v", err)
	}
	if err := RemoveConfigProfile("", "x"); err != nil {
		t.Errorf("expected nil for empty path, got %v", err)
	}
}

func TestRemoveProfile_should_be_noop_when_file_missing(t *testing.T) {
	if err := RemoveProfile(filepath.Join(t.TempDir(), "nope"), "x"); err != nil {
		t.Errorf("expected nil for missing file, got %v", err)
	}
}

func TestCheckPermissions(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credPath, []byte("[x]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tooOpen, mode, err := CheckPermissions(credPath)
	if err != nil {
		t.Fatal(err)
	}
	if !tooOpen {
		t.Errorf("expected 0644 flagged too open, mode=%o", mode)
	}
	if err := os.Chmod(credPath, 0o600); err != nil {
		t.Fatal(err)
	}
	tooOpen, _, _ = CheckPermissions(credPath)
	if tooOpen {
		t.Error("expected 0600 acceptable")
	}
}
