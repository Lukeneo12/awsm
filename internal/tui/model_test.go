package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/runner"
	"github.com/Lukeneo12/awsm/internal/status"
	tea "github.com/charmbracelet/bubbletea"
)

func sampleProfiles() []profiles.Profile {
	return []profiles.Profile{
		{Name: "alpha", Type: profiles.TypeSSO, Region: "us-east-1"},
		{Name: "beta", Type: profiles.TypeManual},
		{Name: "gamma", Type: profiles.TypeSAML, SAMLAccount: "g"},
	}
}

func TestUpdate_status_msg_updates_row(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.checking = 3

	_, _ = m.Update(statusMsg{Profile: "beta", State: status.StateActive, AccountID: "999"})

	if m.statuses["beta"].State != status.StateActive {
		t.Errorf("beta status not recorded")
	}
	if m.checking != 2 {
		t.Errorf("checking counter: got %d want 2", m.checking)
	}
}

func TestUpdate_navigation_wraps(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	if m.cursor != 0 {
		t.Fatalf("cursor should start at 0")
	}
	m.move(-1) // wrap to last
	if m.cursor != 2 {
		t.Errorf("cursor after up-from-top: got %d want 2", m.cursor)
	}
	m.move(1) // wrap to first
	if m.cursor != 0 {
		t.Errorf("cursor after down-from-bottom: got %d want 0", m.cursor)
	}
}

func TestApplyFilter_narrows_list(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.filter = "amm" // matches "gamma"
	m.applyFilter()
	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 match, got %d", len(m.filtered))
	}
	if got, _ := m.selected(); got.Name != "gamma" {
		t.Errorf("selected: got %q want gamma", got.Name)
	}
}

func TestDoSwitch_writes_switch_file(t *testing.T) {
	dir := t.TempDir()
	sf := filepath.Join(dir, "switch")
	m := newModel(runner.NewFake(), sampleProfiles(), sf)
	m.cursor = 1 // beta

	_, cmd := m.doSwitch()
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	// Quit cmd returns a QuitMsg.
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected QuitMsg from switch")
	}

	data, err := os.ReadFile(sf)
	if err != nil {
		t.Fatalf("switch file not written: %v", err)
	}
	if string(data) != "beta" {
		t.Errorf("switch file content: got %q want beta", data)
	}
}

func TestDoSwitch_without_wrapper_shows_hint(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	_, cmd := m.doSwitch()
	if cmd != nil {
		t.Error("expected no quit when wrapper missing")
	}
	if m.message == "" {
		t.Error("expected a hint message when switch file absent")
	}
}

func TestDoLogin_noop_for_keys(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1 // beta (keys)
	_, cmd := m.doLogin()
	if cmd != nil {
		t.Error("keys profile should not trigger an exec command")
	}
}

func TestView_renders_without_panic(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	if out := m.View(); out == "" {
		t.Error("View returned empty string")
	}
}

func TestView_renders_all_badge_states(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.statuses["alpha"] = status.Status{Profile: "alpha", State: status.StateActive, AccountID: "111111111111"}
	m.statuses["beta"] = status.Status{Profile: "beta", State: status.StateExpired}
	m.statuses["gamma"] = status.Status{Profile: "gamma", State: status.StateInvalid}
	m.message = "hello"
	m.filter = "a"
	out := m.View()
	for _, want := range []string{"active", "expired", "invalid", "111111111111"} {
		if !contains(out, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

func key(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestUpdate_q_quits(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	_, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("expected QuitMsg")
	}
}

func TestUpdate_r_refreshes(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	_, cmd := m.Update(key("r"))
	if cmd == nil {
		t.Error("expected a refresh command")
	}
	if m.checking != len(sampleProfiles()) {
		t.Errorf("checking counter: got %d want %d", m.checking, len(sampleProfiles()))
	}
}

func TestUpdate_arrow_navigation(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("down"))
	if m.cursor != 1 {
		t.Errorf("cursor after down: got %d want 1", m.cursor)
	}
	m.Update(key("up"))
	if m.cursor != 0 {
		t.Errorf("cursor after up: got %d want 0", m.cursor)
	}
}

func TestUpdate_slash_enters_filter_then_types(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("/"))
	m.Update(key("b")) // should filter, matching "beta"
	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 filtered match, got %d", len(m.filtered))
	}
	if got, _ := m.selected(); got.Name != "beta" {
		t.Errorf("filtered selection: got %q want beta", got.Name)
	}
	// esc clears the filter
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.filtered) != len(sampleProfiles()) {
		t.Errorf("esc should clear filter, got %d rows", len(m.filtered))
	}
}

func TestDoLogin_returns_exec_for_sso(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 0 // alpha (sso)
	_, cmd := m.doLogin()
	if cmd == nil {
		t.Error("expected an exec command for an sso profile")
	}
}

func TestDoLogin_resolves_role_source(t *testing.T) {
	list := []profiles.Profile{
		{Name: "prod", Type: profiles.TypeRole, SourceProfile: "base"},
		{Name: "base", Type: profiles.TypeSAML, SAMLAccount: "default"},
	}
	m := newModel(runner.NewFake(), list, "")
	m.cursor = 0 // prod (role -> base saml)
	_, cmd := m.doLogin()
	if cmd == nil {
		t.Error("expected an exec command resolving role to saml source")
	}
}

func TestFilter_enter_keeps_filter(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("/"))
	m.Update(key("a")) // matches alpha, beta, gamma
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filter != "a" {
		t.Errorf("filter after enter: got %q want a", m.filter)
	}
}

func TestUpdate_a_launches_add(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	_, cmd := m.Update(key("a"))
	if cmd == nil {
		t.Error("expected an exec command to launch the add wizard")
	}
}

func TestUpdate_t_launches_settype(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1 // beta
	_, cmd := m.Update(key("t"))
	if cmd == nil {
		t.Error("expected an exec command to launch set-type")
	}
}

func TestUpdate_l_opens_paste(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1 // beta
	m.Update(key("l"))
	if m.loadStep != loadPaste {
		t.Fatalf("expected loadPaste, got %v", m.loadStep)
	}
	if m.loadProfile != "beta" {
		t.Errorf("expected target beta, got %q", m.loadProfile)
	}
}

func TestLoad_paste_preview_then_confirm(t *testing.T) {
	dir := t.TempDir()
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = profiles.Paths{
		Credentials: filepath.Join(dir, "credentials"),
		Config:      filepath.Join(dir, "config"),
		Overrides:   filepath.Join(dir, "profiles.ini"),
	}
	m.cursor = 1 // beta

	m.Update(key("l")) // open paste
	m.ta.SetValue("export AWS_ACCESS_KEY_ID=ASIACLIP0001\nexport AWS_SECRET_ACCESS_KEY=sec\n")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD}) // submit for preview

	if m.loadStep != loadConfirm {
		t.Fatalf("expected loadConfirm after ctrl+d, got %v", m.loadStep)
	}
	if m.loadParsed.AccessKeyID != "ASIACLIP0001" {
		t.Errorf("parsed access key: got %q", m.loadParsed.AccessKeyID)
	}
	// preview must not leak the secret
	if contains(m.confirmView(), "sec") && !contains(m.confirmView(), "oculto") {
		t.Error("confirm view should not show the secret")
	}

	_, cmd := m.Update(key("y")) // confirm
	if m.loadStep != loadNone {
		t.Error("load flow should end after confirm")
	}
	if cmd == nil {
		t.Error("expected a reload command after a successful load")
	}
	list, _ := profiles.List(m.paths)
	if p, _ := profiles.Find(list, "beta"); p.AccessKeyIDMasked != "****0001" {
		t.Errorf("expected beta credentials loaded, got %q", p.AccessKeyIDMasked)
	}
}

func TestLoad_bad_paste_stays_in_paste(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1
	m.Update(key("l"))
	m.ta.SetValue("this is not credentials")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.loadStep != loadPaste {
		t.Error("expected to stay in paste mode on bad input")
	}
	if !contains(m.message, "no encontré") {
		t.Errorf("expected a helpful error, got %q", m.message)
	}
}

func TestLoad_esc_cancels_paste(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1
	m.Update(key("l"))
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.loadStep != loadNone {
		t.Error("esc should cancel the load")
	}
	if !contains(m.message, "cancel") {
		t.Errorf("expected cancel message, got %q", m.message)
	}
}

func TestReload_refreshes_from_paths(t *testing.T) {
	dir := t.TempDir()
	ovPath := filepath.Join(dir, "profiles.ini")
	if err := profiles.SetOverride(ovPath, "newone", profiles.Override{Type: profiles.TypeManual}); err != nil {
		t.Fatal(err)
	}
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = profiles.Paths{Overrides: ovPath}

	if cmd := m.reload(); cmd == nil {
		t.Error("expected a check command after reload")
	}
	if _, ok := profiles.Find(m.profiles, "newone"); !ok {
		t.Error("reload should have picked up the new override profile")
	}
}

func TestUpdate_loginDone_triggers_recheck(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	_, cmd := m.Update(loginDoneMsg{profile: "alpha", err: nil})
	if cmd == nil {
		t.Error("expected a recheck command after login")
	}
	if m.checking != 1 {
		t.Errorf("checking: got %d want 1", m.checking)
	}
}
