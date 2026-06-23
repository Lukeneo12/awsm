package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/prompt"
	"github.com/Lukeneo12/awsm/internal/runner"
)

func testApp(r runner.CommandRunner) *app {
	return &app{
		paths: profiles.Paths{
			Config:      filepath.Join("..", "testdata", "config"),
			Credentials: filepath.Join("..", "testdata", "credentials"),
			Saml2aws:    filepath.Join("..", "testdata", "saml2aws"),
		},
		runner: r,
	}
}

func TestListCmd_lists_profiles(t *testing.T) {
	a := testApp(runner.NewFake())
	c := a.listCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	if err := c.Execute(); err != nil {
		t.Fatalf("list error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"PROFILE", "sso-dev", "sso", "base-saml", "saml", "static-keys", "keys", "role-prod", "role"} {
		if !strings.Contains(got, want) {
			t.Errorf("list output missing %q:\n%s", want, got)
		}
	}
}

func TestStatusCmd_reports_states(t *testing.T) {
	f := runner.NewFake()
	f.Responses["aws sts get-caller-identity --profile static-keys"] = runner.Result{
		Stdout: []byte(`{"Account":"123456789012"}`),
	}
	f.DefaultResult = runner.Result{ExitCode: 255, Stderr: []byte("Token has expired")}

	a := testApp(f)
	c := a.statusCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"static-keys"})
	if err := c.Execute(); err != nil {
		t.Fatalf("status error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "static-keys") || !strings.Contains(got, "active") {
		t.Errorf("status output unexpected:\n%s", got)
	}
	if !strings.Contains(got, "123456789012") {
		t.Errorf("status output missing account id:\n%s", got)
	}
}

func TestStatusCmd_unknown_profile_errors(t *testing.T) {
	a := testApp(runner.NewFake())
	c := a.statusCmd()
	c.SetArgs([]string{"ghost-profile"})
	if err := c.Execute(); err == nil {
		t.Error("expected error for unknown profile")
	}
}

func TestSwitchCmd_emits_export(t *testing.T) {
	a := testApp(runner.NewFake())
	c := a.switchCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"sso-dev", "--shell", "bash"})
	if err := c.Execute(); err != nil {
		t.Fatalf("switch error: %v", err)
	}
	if !strings.Contains(out.String(), "export AWS_PROFILE=sso-dev") {
		t.Errorf("switch output: %q", out.String())
	}
}

func TestAddManualWizardAndRm_roundtrip(t *testing.T) {
	dir := t.TempDir()
	a := &app{
		paths: profiles.Paths{
			Credentials: filepath.Join(dir, "credentials"),
			Config:      filepath.Join(dir, "config"),
			Overrides:   filepath.Join(dir, "profiles.ini"),
		},
		runner: runner.NewFake(),
	}

	// Wizard (manual) reads: access key id, secret, session token, region.
	add := a.addCmd()
	add.SetIn(strings.NewReader("ASIATEST00009999\ntopsecretvalue\n\nus-east-2\n"))
	add.SetArgs([]string{"newprof", "--type", "manual"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add error: %v", err)
	}

	list, err := profiles.List(a.paths)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := profiles.Find(list, "newprof"); !ok || p.Type != profiles.TypeManual {
		t.Fatalf("expected newprof as manual, got %+v (ok=%v)", p, ok)
	}

	rm := a.rmCmd()
	rm.SetArgs([]string{"newprof"})
	if err := rm.Execute(); err != nil {
		t.Fatalf("rm error: %v", err)
	}
	// override remains but credentials section is gone -> still classified manual
	// only if override present; clear it too via set-type empty.
	list, _ = profiles.List(a.paths)
	if p, _ := profiles.Find(list, "newprof"); p.AccessKeyIDMasked != "" {
		t.Error("credentials should have been removed")
	}
}

func TestRmCmd_fully_forgets_profile(t *testing.T) {
	dir := t.TempDir()
	a := &app{
		paths: profiles.Paths{
			Credentials: filepath.Join(dir, "credentials"),
			Config:      filepath.Join(dir, "config"),
			Overrides:   filepath.Join(dir, "profiles.ini"),
		},
		runner: runner.NewFake(),
	}

	// add a manual profile (writes credentials + config region + override)
	add := a.addCmd()
	add.SetIn(strings.NewReader("ASIA0001\nsecret\n\nus-east-1\n"))
	add.SetArgs([]string{"temp", "--type", "manual"})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}

	// rm should leave no trace (not even an "unknown" leftover from config)
	rm := a.rmCmd()
	rm.SetArgs([]string{"temp"})
	if err := rm.Execute(); err != nil {
		t.Fatalf("rm error: %v", err)
	}

	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "temp"); ok {
		t.Error("profile should be fully gone after rm (no config/override leftover)")
	}
}

func TestAddRoleWizard(t *testing.T) {
	dir := t.TempDir()
	a := &app{
		paths: profiles.Paths{
			Config:    filepath.Join(dir, "config"),
			Overrides: filepath.Join(dir, "profiles.ini"),
		},
		runner: runner.NewFake(),
	}
	add := a.addCmd()
	add.SetIn(strings.NewReader("arn:aws:iam::1:role/r\nbase\nus-east-1\n"))
	add.SetArgs([]string{"prod", "--type", "role"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add role error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	if p, _ := profiles.Find(list, "prod"); p.Type != profiles.TypeRole {
		t.Errorf("expected role, got %+v", p)
	}
}

func TestAddSSOWizard(t *testing.T) {
	dir := t.TempDir()
	a := &app{
		paths:  profiles.Paths{Config: filepath.Join(dir, "config"), Overrides: filepath.Join(dir, "p.ini")},
		runner: runner.NewFake(),
	}
	add := a.addCmd()
	// session, starturl, ssoregion, account, role, region
	add.SetIn(strings.NewReader("corp\nhttps://corp.awsapps.com/start\nus-east-1\n123456789012\nAdmin\nus-east-1\n"))
	add.SetArgs([]string{"mysso", "--type", "sso"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add sso error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	if p, _ := profiles.Find(list, "mysso"); p.Type != profiles.TypeSSO {
		t.Errorf("expected sso, got %+v", p)
	}
}

func TestAddSAMLWizard(t *testing.T) {
	dir := t.TempDir()
	a := &app{
		paths: profiles.Paths{
			Saml2aws:  filepath.Join(dir, "saml2aws"),
			Overrides: filepath.Join(dir, "p.ini"),
		},
		runner: runner.NewFake(),
	}
	add := a.addCmd()
	// account, url, provider, mfa, rolearn, awsprofile, region
	add.SetIn(strings.NewReader("acme\nhttps://idp\nGoogleApps\nAuto\narn:aws:iam::1:role/r\nacme-prof\nus-east-1\n"))
	add.SetArgs([]string{"acme", "--type", "saml"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add saml error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	if p, _ := profiles.Find(list, "acme-prof"); p.Type != profiles.TypeSAML {
		t.Errorf("expected saml, got %+v", p)
	}
}

func TestAddCmd_prompts_for_name_when_absent(t *testing.T) {
	dir := t.TempDir()
	a := &app{
		paths: profiles.Paths{
			Config:    filepath.Join(dir, "config"),
			Overrides: filepath.Join(dir, "p.ini"),
		},
		runner: runner.NewFake(),
	}
	add := a.addCmd()
	// profile name prompt, then role fields
	add.SetIn(strings.NewReader("prod\narn:aws:iam::1:role/r\nbase\nus-east-1\n"))
	add.SetArgs([]string{"--type", "role"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "prod"); !ok {
		t.Error("expected prod profile created via prompted name")
	}
}

func TestAddCmd_invalid_type(t *testing.T) {
	a := testApp(runner.NewFake())
	add := a.addCmd()
	add.SetArgs([]string{"x", "--type", "bogus"})
	if err := add.Execute(); err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestSetTypeCmd_pins_and_clears(t *testing.T) {
	dir := t.TempDir()
	ovPath := filepath.Join(dir, "profiles.ini")
	a := &app{
		paths: profiles.Paths{
			Config:      filepath.Join("..", "testdata", "config"),
			Credentials: filepath.Join("..", "testdata", "credentials"),
			Saml2aws:    filepath.Join("..", "testdata", "saml2aws"),
			Overrides:   ovPath,
		},
		runner: runner.NewFake(),
	}

	// base-saml auto-detects as saml; pin it to manual.
	st := a.setTypeCmd()
	st.SetArgs([]string{"base-saml", "manual"})
	if err := st.Execute(); err != nil {
		t.Fatalf("set-type error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	if p, _ := profiles.Find(list, "base-saml"); p.Type != profiles.TypeManual {
		t.Errorf("expected manual after pin, got %q", p.Type)
	}

	// clear -> back to saml
	clear := a.setTypeCmd()
	clear.SetArgs([]string{"base-saml", "--clear"})
	if err := clear.Execute(); err != nil {
		t.Fatalf("clear error: %v", err)
	}
	list, _ = profiles.List(a.paths)
	if p, _ := profiles.Find(list, "base-saml"); p.Type != profiles.TypeSAML {
		t.Errorf("expected saml after clear, got %q", p.Type)
	}
}

func TestSetTypeCmd_invalid(t *testing.T) {
	a := testApp(runner.NewFake())
	a.paths.Overrides = filepath.Join(t.TempDir(), "p.ini")
	st := a.setTypeCmd()
	st.SetArgs([]string{"x", "nonsense"})
	if err := st.Execute(); err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestLoginCmd_dispatches_sso(t *testing.T) {
	f := runner.NewFake()
	a := testApp(f)
	c := a.loginCmd()
	c.SetArgs([]string{"sso-dev"})
	if err := c.Execute(); err != nil {
		t.Fatalf("login error: %v", err)
	}
	if len(f.InteractiveCalls) != 1 || f.InteractiveCalls[0].Name != "aws" {
		t.Errorf("expected one aws login call, got %+v", f.InteractiveCalls)
	}
}

func TestLoginCmd_noop_for_keys(t *testing.T) {
	f := runner.NewFake()
	a := testApp(f)
	c := a.loginCmd()
	c.SetArgs([]string{"static-keys"})
	if err := c.Execute(); err != nil {
		t.Fatalf("login error: %v", err)
	}
	if len(f.InteractiveCalls) != 0 {
		t.Error("keys profile should not trigger a login")
	}
}

func TestLoginCmd_unknown_profile_errors(t *testing.T) {
	a := testApp(runner.NewFake())
	c := a.loginCmd()
	c.SetArgs([]string{"ghost"})
	if err := c.Execute(); err == nil {
		t.Error("expected error for unknown profile")
	}
}

func loadApp(t *testing.T, confirm func(string) (bool, error)) *app {
	t.Helper()
	dir := t.TempDir()
	return &app{
		paths: profiles.Paths{
			Credentials: filepath.Join(dir, "credentials"),
			Config:      filepath.Join(dir, "config"),
			Overrides:   filepath.Join(dir, "profiles.ini"),
		},
		runner:  runner.NewFake(),
		confirm: confirm,
	}
}

func TestLoadCredsCmd_confirm_saves(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return true, nil })
	c := a.loadCredsCmd()
	c.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=\"ASIAEXAMPLE9999\"\n" +
			"export AWS_SECRET_ACCESS_KEY=\"thesecret\"\n" +
			"export AWS_SESSION_TOKEN=\"thetoken\"\n" +
			"export AWS_DEFAULT_REGION=\"us-east-1\"\n"))
	c.SetArgs([]string{"dino-dev"})
	if err := c.Execute(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	p, ok := profiles.Find(list, "dino-dev")
	if !ok || p.Type != profiles.TypeManual {
		t.Fatalf("expected dino-dev manual, got %+v (ok=%v)", p, ok)
	}
	if p.AccessKeyIDMasked != "****9999" {
		t.Errorf("masked key: got %q", p.AccessKeyIDMasked)
	}
}

func TestLoadCredsCmd_decline_writes_nothing(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return false, nil })
	c := a.loadCredsCmd()
	c.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=ASIA9999\nexport AWS_SECRET_ACCESS_KEY=s\n"))
	c.SetArgs([]string{"dino-dev"})
	if err := c.Execute(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "dino-dev"); ok {
		t.Error("declining should write nothing")
	}
}

func TestLoadCredsCmd_non_interactive_auto_confirms(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return false, prompt.ErrNoTTY })
	c := a.loadCredsCmd()
	c.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=AKIA1234\nexport AWS_SECRET_ACCESS_KEY=s\n"))
	c.SetArgs([]string{"ci-prof"})
	if err := c.Execute(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	list, _ := profiles.List(a.paths)
	if p, ok := profiles.Find(list, "ci-prof"); !ok || p.Type != profiles.TypeManual {
		t.Fatalf("non-interactive run should save, got ok=%v", ok)
	}
}

func TestLoadCredsCmd_bad_content_errors(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return true, nil })
	c := a.loadCredsCmd()
	c.SetIn(strings.NewReader("not credentials at all"))
	c.SetArgs([]string{"x"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when input has no credentials")
	}
}

// The two security invariants the command promises: the secret/session token is
// never printed (only ****last4 of the access key id), and stdout stays
// eval-safe (every diagnostic goes to stderr). Both are captured via cobra's
// output seams so a future regression turns this test red.
func TestLoadCredsCmd_does_not_leak_secret(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return true, nil })
	c := a.loadCredsCmd()
	var errBuf, outBuf bytes.Buffer
	c.SetErr(&errBuf)
	c.SetOut(&outBuf)
	c.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=\"ASIAEXAMPLE9999\"\n" +
			"export AWS_SECRET_ACCESS_KEY=\"thesecret\"\n" +
			"export AWS_SESSION_TOKEN=\"thetoken\"\n" +
			"export AWS_DEFAULT_REGION=\"us-east-1\"\n"))
	c.SetArgs([]string{"dino-dev"})
	if err := c.Execute(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	errOut := errBuf.String()
	if strings.Contains(errOut, "thesecret") || strings.Contains(errOut, "thetoken") {
		t.Errorf("secret/session token leaked to stderr: %q", errOut)
	}
	if !strings.Contains(errOut, "****9999") {
		t.Errorf("masked key preview missing from stderr: %q", errOut)
	}
	if outBuf.Len() != 0 {
		t.Errorf("stdout must stay eval-safe (empty), got %q", outBuf.String())
	}
}

func TestShellInitCmd_emits_wrapper(t *testing.T) {
	a := testApp(runner.NewFake())
	c := a.shellInitCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"zsh"})
	if err := c.Execute(); err != nil {
		t.Fatalf("shell-init error: %v", err)
	}
	if !strings.Contains(out.String(), "awsm()") {
		t.Errorf("shell-init output: %q", out.String())
	}
}
