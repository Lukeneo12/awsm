package cmd

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/prompt"
	"github.com/Lukeneo12/awsm/internal/runner"
	"gopkg.in/ini.v1"
)

// eofThenReader simulates a live terminal's EOF semantics for tests: the
// first Read returns an immediate (0, io.EOF) -- as if the user pressed the
// EOF key on an empty line, submitting an empty paste -- and every call after
// that serves bytes from rest. A plain strings.Reader/bytes.Reader EOFs
// permanently once drained and cannot represent "empty paste, then more
// input"; addManual's paste-then-fallback path needs exactly that sequence,
// and relies on bufio.Reader retrying the underlying reader after an EOF
// (bufio.Reader clears its cached error once it has been returned), which is
// what makes a real TTY's Ctrl+D-then-keep-typing behavior work too.
type eofThenReader struct {
	usedEOF bool
	rest    io.Reader
}

func (r *eofThenReader) Read(p []byte) (int, error) {
	if !r.usedEOF {
		r.usedEOF = true
		return 0, io.EOF
	}
	return r.rest.Read(p)
}

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

	// An empty paste (immediate EOF) falls back to the field-by-field wizard:
	// access key id, secret, session token, region.
	add := a.addCmd()
	add.SetIn(&eofThenReader{rest: strings.NewReader("ASIATEST00009999\ntopsecretvalue\n\nus-east-2\n")})
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

	// add a manual profile via the field-by-field fallback (empty paste)
	// (writes credentials + config region + override)
	add := a.addCmd()
	add.SetIn(&eofThenReader{rest: strings.NewReader("ASIA0001\nsecret\n\nus-east-1\n")})
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

// A runaway pipe must fail loudly rather than be silently truncated and parsed.
func TestLoadCredsCmd_rejects_oversized_input(t *testing.T) {
	a := loadApp(t, func(string) (bool, error) { return true, nil })
	c := a.loadCredsCmd()
	// A valid-looking prefix followed by enough padding to blow past the 1 MiB
	// cap; the size guard must fire before we ever parse or write.
	big := "export AWS_ACCESS_KEY_ID=AKIA1234\nexport AWS_SECRET_ACCESS_KEY=s\n" +
		strings.Repeat("#", 1<<20)
	c.SetIn(strings.NewReader(big))
	c.SetArgs([]string{"too-big"})
	if err := c.Execute(); err == nil {
		t.Fatal("expected error for oversized input")
	}
	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "too-big"); ok {
		t.Error("oversized input must not write a profile")
	}
}

// addManualApp builds an app wired to a temp dir, mirroring loadApp, so the
// add --type manual paste tests can inject a and confirm the same way the
// load-credentials tests do.
func addManualApp(t *testing.T, confirm func(string) (bool, error)) *app {
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

// storedSessionToken reads the raw aws_session_token value straight out of
// the credentials ini file, bypassing profiles.List's masked view, so tests
// can assert the token was stored byte-for-byte intact.
func storedSessionToken(t *testing.T, credentialsPath, profile string) string {
	t.Helper()
	f, err := ini.Load(credentialsPath)
	if err != nil {
		t.Fatalf("reading credentials file: %v", err)
	}
	sec, err := f.GetSection(profile)
	if err != nil {
		t.Fatalf("section %q missing: %v", profile, err)
	}
	return sec.Key("aws_session_token").String()
}

// AC1: a valid paste, including a long session token, is parsed, previewed
// and stored intact on confirm.
func TestAddManualCmd_valid_paste_stores_session_token_intact(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return true, nil })
	longToken := "IQoJb3JpZ2luX2VjE" + strings.Repeat("aB3", 200) + "ZXhhbXBsZXRva2VuZW5k"

	add := a.addCmd()
	add.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=\"ASIAEXAMPLE9999\"\n" +
			"export AWS_SECRET_ACCESS_KEY=\"thesecret\"\n" +
			"export AWS_SESSION_TOKEN=\"" + longToken + "\"\n" +
			"export AWS_DEFAULT_REGION=\"us-east-1\"\n"))
	add.SetArgs([]string{"dev", "--type", "manual"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add error: %v", err)
	}

	list, _ := profiles.List(a.paths)
	p, ok := profiles.Find(list, "dev")
	if !ok || p.Type != profiles.TypeManual {
		t.Fatalf("expected dev manual, got %+v (ok=%v)", p, ok)
	}
	if p.AccessKeyIDMasked != "****9999" {
		t.Errorf("masked key: got %q", p.AccessKeyIDMasked)
	}
	if got := storedSessionToken(t, a.paths.Credentials, "dev"); got != longToken {
		t.Errorf("session token not stored intact:\n got  %q\n want %q", got, longToken)
	}
}

// AC2: an empty paste (immediate EOF) falls back to the field-by-field
// wizard, unchanged.
func TestAddManualCmd_empty_paste_falls_back_to_field_by_field(t *testing.T) {
	a := addManualApp(t, nil)

	add := a.addCmd()
	add.SetIn(&eofThenReader{rest: strings.NewReader("ASIAFIELDS0001\nfieldsecret\n\nus-west-2\n")})
	add.SetArgs([]string{"fields-prof", "--type", "manual"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add error: %v", err)
	}

	list, _ := profiles.List(a.paths)
	p, ok := profiles.Find(list, "fields-prof")
	if !ok || p.Type != profiles.TypeManual {
		t.Fatalf("expected fields-prof manual, got %+v (ok=%v)", p, ok)
	}
	if p.AccessKeyIDMasked != "****0001" {
		t.Errorf("masked key: got %q", p.AccessKeyIDMasked)
	}
}

// AC3: a non-empty but unparseable paste errors and does not fall back to
// field-by-field with a half-consumed paste.
func TestAddManualCmd_unparseable_paste_errors_and_writes_nothing(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return true, nil })

	add := a.addCmd()
	add.SetIn(strings.NewReader("this is not a credentials block at all"))
	add.SetArgs([]string{"bad-paste", "--type", "manual"})
	if err := add.Execute(); err == nil {
		t.Fatal("expected error for unparseable paste")
	}

	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "bad-paste"); ok {
		t.Error("unparseable paste must not write a profile")
	}
}

// AC4: input larger than 1 MiB is refused before parsing or writing.
func TestAddManualCmd_rejects_oversized_paste(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return true, nil })

	big := "export AWS_ACCESS_KEY_ID=AKIA1234\nexport AWS_SECRET_ACCESS_KEY=s\n" +
		strings.Repeat("#", 1<<20)
	add := a.addCmd()
	add.SetIn(strings.NewReader(big))
	add.SetArgs([]string{"too-big", "--type", "manual"})
	if err := add.Execute(); err == nil {
		t.Fatal("expected error for oversized paste")
	}

	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "too-big"); ok {
		t.Error("oversized paste must not write a profile")
	}
}

// AC5: an explicit decline on the console writes nothing.
func TestAddManualCmd_decline_confirm_writes_nothing(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return false, nil })

	add := a.addCmd()
	add.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=ASIA9999\nexport AWS_SECRET_ACCESS_KEY=s\n"))
	add.SetArgs([]string{"declined", "--type", "manual"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add error: %v", err)
	}

	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "declined"); ok {
		t.Error("declining should write nothing")
	}
}

// AC5: with no console available (prompt.ErrNoTTY), the save proceeds with a
// notice instead of blocking.
func TestAddManualCmd_non_interactive_auto_confirms(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return false, prompt.ErrNoTTY })

	add := a.addCmd()
	var errBuf bytes.Buffer
	add.SetErr(&errBuf)
	add.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=AKIA1234\nexport AWS_SECRET_ACCESS_KEY=s\n"))
	add.SetArgs([]string{"ci-prof", "--type", "manual"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add error: %v", err)
	}

	list, _ := profiles.List(a.paths)
	if p, ok := profiles.Find(list, "ci-prof"); !ok || p.Type != profiles.TypeManual {
		t.Fatalf("non-interactive run should save, got ok=%v", ok)
	}
	if !strings.Contains(errBuf.String(), "non-interactive: saved without confirmation") {
		t.Errorf("missing non-interactive notice: %q", errBuf.String())
	}
}

// AC6: the secret and session token are never printed (stdout or stderr);
// stdout stays eval-safe. Mirrors TestLoadCredsCmd_does_not_leak_secret.
func TestAddManualCmd_does_not_leak_secret(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return true, nil })

	add := a.addCmd()
	var errBuf, outBuf bytes.Buffer
	add.SetErr(&errBuf)
	add.SetOut(&outBuf)
	add.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=\"ASIAEXAMPLE9999\"\n" +
			"export AWS_SECRET_ACCESS_KEY=\"thesecret\"\n" +
			"export AWS_SESSION_TOKEN=\"thetoken\"\n" +
			"export AWS_DEFAULT_REGION=\"us-east-1\"\n"))
	add.SetArgs([]string{"dino-dev", "--type", "manual"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add error: %v", err)
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

// AC7-adjacent CLI edge case: when both the profile name and the type are
// entered interactively (no positional arg, no --type flag), the paste read
// must still work through the same wizard's bufio.Reader that already
// consumed the name and type lines -- not a fresh cmd.InOrStdin() call, which
// would race the buffered reader for the same fd and lose or duplicate bytes.
func TestAddCmd_interactive_name_and_type_then_paste_share_the_reader(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return true, nil })

	add := a.addCmd()
	// line 1: profile name, line 2: type, remaining bytes: the pasted block.
	add.SetIn(strings.NewReader(
		"interactive-dev\n" +
			"manual\n" +
			"export AWS_ACCESS_KEY_ID=\"ASIAINTER0007\"\n" +
			"export AWS_SECRET_ACCESS_KEY=\"secret\"\n"))
	// No args, no --type: both profile name and type are prompted.
	if err := add.Execute(); err != nil {
		t.Fatalf("add error: %v", err)
	}

	list, _ := profiles.List(a.paths)
	p, ok := profiles.Find(list, "interactive-dev")
	if !ok || p.Type != profiles.TypeManual {
		t.Fatalf("expected interactive-dev manual, got %+v (ok=%v)", p, ok)
	}
	if p.AccessKeyIDMasked != "****0007" {
		t.Errorf("masked key: got %q (paste bytes may have been lost/duplicated across readers)", p.AccessKeyIDMasked)
	}
}

// AC3 edge case: a paste that is non-empty but carries only whitespace and
// comment lines must NOT be treated as an empty paste (AC2's fallback
// trigger is strings.TrimSpace(raw) == "", and comment/whitespace-only text
// does not collapse to ""). It should hit the parse-failure path instead.
func TestAddManualCmd_whitespace_and_comment_only_paste_errors_not_fallback(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return true, nil })

	add := a.addCmd()
	add.SetIn(strings.NewReader("   \n# just a comment, no real fields\n; another comment\n\n"))
	add.SetArgs([]string{"comment-only", "--type", "manual"})
	if err := add.Execute(); err == nil {
		t.Fatal("expected a parse error, not a silent fallback to field-by-field")
	}

	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "comment-only"); ok {
		t.Error("a whitespace/comment-only paste must not write a profile")
	}
}

// AC3: the parse-failure error carries the same paste hint load-credentials
// has always shown, including the EOF key the user is told to press.
func TestAddManualCmd_unparseable_paste_error_mentions_EOF_hint(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return true, nil })

	add := a.addCmd()
	add.SetIn(strings.NewReader("this is not a credentials block at all"))
	add.SetArgs([]string{"bad-paste-hint", "--type", "manual"})
	err := add.Execute()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), prompt.EOFKey) {
		t.Errorf("expected error to mention the EOF key %q, got: %v", prompt.EOFKey, err)
	}
}

// Edge case: a paste using CRLF line endings (as pasted from Windows-authored
// text or some SSO portal pages) must parse the same as LF-only input --
// strings.TrimSpace strips the trailing \r along with other whitespace.
func TestAddManualCmd_crlf_paste_parses(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return true, nil })

	add := a.addCmd()
	add.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=\"ASIACRLF0042\"\r\n" +
			"export AWS_SECRET_ACCESS_KEY=\"secretcrlf\"\r\n" +
			"export AWS_DEFAULT_REGION=\"us-east-1\"\r\n"))
	add.SetArgs([]string{"crlf-prof", "--type", "manual"})
	if err := add.Execute(); err != nil {
		t.Fatalf("add error: %v", err)
	}

	list, _ := profiles.List(a.paths)
	p, ok := profiles.Find(list, "crlf-prof")
	if !ok || p.AccessKeyIDMasked != "****0042" {
		t.Fatalf("expected crlf-prof with masked key ****0042, got %+v (ok=%v)", p, ok)
	}
}

// Edge case: a.confirm returning a plain (non-ErrNoTTY) error must propagate
// out of addManual and abort the write, exactly like load-credentials.
func TestAddManualCmd_confirm_error_propagates_and_writes_nothing(t *testing.T) {
	boom := errors.New("boom: console read failed")
	a := addManualApp(t, func(string) (bool, error) { return false, boom })

	add := a.addCmd()
	add.SetIn(strings.NewReader(
		"export AWS_ACCESS_KEY_ID=AKIA1234\nexport AWS_SECRET_ACCESS_KEY=s\n"))
	add.SetArgs([]string{"confirm-err", "--type", "manual"})
	err := add.Execute()
	if err == nil {
		t.Fatal("expected the confirm error to propagate")
	}
	if !errors.Is(err, boom) {
		t.Errorf("expected wrapped boom error, got: %v", err)
	}

	list, _ := profiles.List(a.paths)
	if _, ok := profiles.Find(list, "confirm-err"); ok {
		t.Error("a confirm error must not write a profile")
	}
}

// Edge case: a reader that fails outright (simulating a broken pipe) must
// surface as a readPastedBlock error rather than being swallowed.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("simulated read failure") }

func TestAddManualCmd_reader_failure_propagates(t *testing.T) {
	a := addManualApp(t, func(string) (bool, error) { return true, nil })

	add := a.addCmd()
	add.SetIn(errReader{})
	add.SetArgs([]string{"reader-fail", "--type", "manual"})
	if err := add.Execute(); err == nil {
		t.Error("expected an error when the underlying reader fails")
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
