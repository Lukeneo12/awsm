package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Lukeneo12/awsm/internal/creds"
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

func TestUpdate_a_opens_type_menu(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	_, cmd := m.Update(key("a"))
	if m.loadStep != loadType {
		t.Fatalf("expected loadType after 'a', got %v", m.loadStep)
	}
	if cmd != nil {
		t.Error("opening the type menu should not launch an exec command yet")
	}
}

func TestAddType_manual_goes_to_name_step(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))                       // type menu
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // cursor at 0 = manual
	if m.loadStep != loadName {
		t.Fatalf("expected loadName after selecting manual, got %v", m.loadStep)
	}
}

func TestAddType_nonmanual_suspends_to_wizard(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))           // type menu
	_, cmd := m.Update(key("2")) // sso -> CLI wizard
	if cmd == nil {
		t.Error("expected an exec command to launch the CLI wizard for sso")
	}
	if m.loadStep != loadNone {
		t.Errorf("expected load flow reset before suspending, got %v", m.loadStep)
	}
}

func TestAddType_1to4_shortcuts_select_each_type(t *testing.T) {
	types := addableTypes()
	for i, want := range types {
		m := newModel(runner.NewFake(), sampleProfiles(), "")
		m.Update(key("a"))
		shortcut := string(rune('1' + i))
		_, cmd := m.Update(key(shortcut))
		if want == profiles.TypeManual {
			if m.loadStep != loadName {
				t.Errorf("shortcut %q: expected loadName for manual, got %v", shortcut, m.loadStep)
			}
			continue
		}
		if cmd == nil {
			t.Errorf("shortcut %q: expected an exec command for %s", shortcut, want)
		}
	}
}

func TestAddType_navigates_with_arrows_and_wraps(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))
	m.Update(key("down"))
	if m.typeCursor != 1 {
		t.Errorf("type cursor after down: got %d want 1", m.typeCursor)
	}
	m.Update(key("up"))
	m.Update(key("up")) // wraps to the last entry
	if m.typeCursor != len(addableTypes())-1 {
		t.Errorf("type cursor after wrap: got %d want %d", m.typeCursor, len(addableTypes())-1)
	}
}

func TestAddType_esc_cancels_back_to_list(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))
	m.Update(key("down"))
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.loadStep != loadNone {
		t.Error("esc should cancel the type menu")
	}
	if !contains(m.message, "cancel") {
		t.Errorf("expected a cancel message, got %q", m.message)
	}
}

func TestAdd_name_then_paste_creates_manual_profile(t *testing.T) {
	dir := t.TempDir()
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = profiles.Paths{
		Credentials: filepath.Join(dir, "credentials"),
		Config:      filepath.Join(dir, "config"),
		Overrides:   filepath.Join(dir, "profiles.ini"),
	}

	m.Update(key("a"))                       // type menu
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // manual -> name step
	for _, r := range "delta" {
		m.Update(key(string(r)))
	}
	if m.nameInput != "delta" {
		t.Fatalf("name buffer: got %q want delta", m.nameInput)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // name -> paste
	if m.loadStep != loadPaste || m.loadProfile != "delta" {
		t.Fatalf("expected loadPaste for delta, got step=%v profile=%q", m.loadStep, m.loadProfile)
	}

	m.ta.SetValue("export AWS_ACCESS_KEY_ID=ASIANEW00001\n" +
		"export AWS_SECRET_ACCESS_KEY=sec\nexport AWS_SESSION_TOKEN=tok\n")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD}) // preview
	if m.loadStep != loadConfirm {
		t.Fatalf("expected loadConfirm, got %v", m.loadStep)
	}
	m.Update(key("y")) // confirm -> store

	list, _ := profiles.List(m.paths)
	p, ok := profiles.Find(list, "delta")
	if !ok || p.Type != profiles.TypeManual {
		t.Fatalf("expected delta stored as manual, got %+v (ok=%v)", p, ok)
	}
	if p.AccessKeyIDMasked != "****0001" {
		t.Errorf("masked key: got %q", p.AccessKeyIDMasked)
	}

	info, err := os.Stat(m.paths.Credentials)
	if err != nil {
		t.Fatalf("stat credentials: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("credentials file mode: got %o want 0600", info.Mode().Perm())
	}
}

func TestAdd_name_rejects_empty_name(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))                       // type menu
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // manual -> name step

	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // empty name

	if m.loadStep != loadName {
		t.Error("empty name should keep the name step")
	}
	if !contains(m.message, "vacío") {
		t.Errorf("expected empty-name message, got %q", m.message)
	}
}

func TestAdd_name_rejects_duplicate_name(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))                       // type menu
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // manual -> name step

	for _, r := range "beta" { // beta already exists
		m.Update(key(string(r)))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.loadStep != loadName {
		t.Error("duplicate name should keep the name step")
	}
	if !contains(m.message, "ya existe") {
		t.Errorf("expected duplicate message, got %q", m.message)
	}
}

func TestAdd_name_backspace_edits_buffer(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(key("d"))
	m.Update(key("x"))
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.nameInput != "d" {
		t.Errorf("name buffer after backspace: got %q want %q", m.nameInput, "d")
	}
}

func TestAdd_name_esc_cancels(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))                       // type menu
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // manual -> name step
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})   // cancel from name step
	if m.loadStep != loadNone {
		t.Error("esc should cancel the name step")
	}
	if !contains(m.message, "cancel") {
		t.Errorf("expected a cancel message, got %q", m.message)
	}
}

func TestView_renders_add_type_and_name_steps(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")

	m.loadStep = loadType
	if out := m.View(); !contains(out, "manual") || !contains(out, "sso") {
		t.Errorf("type view missing options:\n%s", out)
	}

	m.loadStep = loadName
	m.nameInput = "delta"
	if out := m.View(); !contains(out, "delta") {
		t.Errorf("name view should echo the typed name:\n%s", out)
	}
}

// 'd' during the type menu must be ignored, not start a delete.
func TestUpdate_d_during_type_menu_is_ignored(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))
	m.Update(key("d"))
	if m.deleteTarget != "" {
		t.Errorf("expected 'd' during the type menu to not start a delete, got deleteTarget %q", m.deleteTarget)
	}
	if m.loadStep != loadType {
		t.Error("expected to remain on the type menu")
	}
}

// 'd' during the name step must be typed into the name buffer, not start a
// delete.
func TestUpdate_d_during_name_step_is_typed_not_delete(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // manual -> name step

	m.Update(key("d"))

	if m.deleteTarget != "" {
		t.Errorf("expected 'd' during the name step to not start a delete, got deleteTarget %q", m.deleteTarget)
	}
	if m.loadStep != loadName {
		t.Error("expected to remain on the name step")
	}
	if m.nameInput != "d" {
		t.Errorf("expected 'd' to be typed into the name buffer, got %q", m.nameInput)
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

func newDeletePaths(dir string) profiles.Paths {
	return profiles.Paths{
		Credentials: filepath.Join(dir, "credentials"),
		Config:      filepath.Join(dir, "config"),
		Overrides:   filepath.Join(dir, "profiles.ini"),
	}
}

// seedManualProfile writes a manual profile + override into fixture files so a
// delete test has something real to remove.
func seedManualProfile(t *testing.T, paths profiles.Paths, name string) {
	t.Helper()
	in := profiles.ManualInput{AccessKeyID: "AKIASEED0001", Secret: "sekret", Region: "us-east-1"}
	if err := profiles.AddManual(paths.Credentials, paths.Config, name, in); err != nil {
		t.Fatalf("seed AddManual: %v", err)
	}
	if err := profiles.SetOverride(paths.Overrides, name, profiles.Override{Type: profiles.TypeManual}); err != nil {
		t.Fatalf("seed SetOverride: %v", err)
	}
}

func TestUpdate_d_shows_delete_confirm(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1 // beta
	m.Update(key("d"))
	if m.deleteTarget != "beta" {
		t.Fatalf("expected deleteTarget beta, got %q", m.deleteTarget)
	}
	if !contains(m.View(), "Delete profile") {
		t.Error("expected the delete confirm screen to render")
	}
}

func TestUpdate_d_on_empty_list_is_noop(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.filter = "zzz-no-match"
	m.applyFilter()
	if len(m.filtered) != 0 {
		t.Fatalf("expected empty filtered list, got %d", len(m.filtered))
	}
	_, cmd := m.Update(key("d"))
	if cmd != nil {
		t.Error("expected no command when list is empty")
	}
	if m.deleteTarget != "" {
		t.Errorf("expected no pending delete, got %q", m.deleteTarget)
	}
}

func TestDeleteConfirm_y_removes_profile_and_reloads(t *testing.T) {
	dir := t.TempDir()
	paths := newDeletePaths(dir)
	seedManualProfile(t, paths, "beta")
	seedManualProfile(t, paths, "alpha") // survives, so reload has something to check

	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = paths
	m.deleteTarget = "beta"

	_, cmd := m.Update(key("y"))
	if m.deleteTarget != "" {
		t.Error("deleteTarget should be cleared after confirm")
	}
	if !contains(m.message, "deleted") {
		t.Errorf("expected a success message, got %q", m.message)
	}
	if cmd == nil {
		t.Fatal("expected a reload command after a successful delete")
	}
	if _, ok := profiles.Find(m.profiles, "beta"); ok {
		t.Error("expected beta to be gone from the reloaded profile list")
	}

	credData, err := os.ReadFile(paths.Credentials)
	if err != nil {
		t.Fatalf("reading credentials fixture: %v", err)
	}
	if contains(string(credData), "beta") {
		t.Error("expected the beta section to be removed from credentials")
	}
	configData, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatalf("reading config fixture: %v", err)
	}
	if contains(string(configData), "beta") {
		t.Error("expected the beta section to be removed from config")
	}
	overrides, err := profiles.LoadOverrides(paths.Overrides)
	if err != nil {
		t.Fatalf("reading overrides fixture: %v", err)
	}
	if _, ok := overrides["beta"]; ok {
		t.Error("expected the beta override to be cleared")
	}
}

func TestDeleteConfirm_y_keeps_credentials_file_mode_0600(t *testing.T) {
	dir := t.TempDir()
	paths := newDeletePaths(dir)
	seedManualProfile(t, paths, "beta")

	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = paths
	m.deleteTarget = "beta"
	m.Update(key("y"))

	info, err := os.Stat(paths.Credentials)
	if err != nil {
		t.Fatalf("stat credentials: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("credentials file mode: got %o want 0600", info.Mode().Perm())
	}
}

func TestDeleteConfirm_n_cancels_without_deleting(t *testing.T) {
	dir := t.TempDir()
	paths := newDeletePaths(dir)
	seedManualProfile(t, paths, "beta")

	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = paths
	m.deleteTarget = "beta"

	_, cmd := m.Update(key("n"))
	if cmd != nil {
		t.Error("expected no command on cancel")
	}
	if m.deleteTarget != "" {
		t.Error("deleteTarget should be cleared on cancel")
	}
	if !contains(m.message, "cancel") {
		t.Errorf("expected a cancelled message, got %q", m.message)
	}
	credData, _ := os.ReadFile(paths.Credentials)
	if !contains(string(credData), "beta") {
		t.Error("beta section should still exist in credentials after cancel")
	}
}

func TestDeleteConfirm_esc_cancels_without_deleting(t *testing.T) {
	dir := t.TempDir()
	paths := newDeletePaths(dir)
	seedManualProfile(t, paths, "beta")

	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = paths
	m.deleteTarget = "beta"

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.deleteTarget != "" {
		t.Error("deleteTarget should be cleared on esc")
	}
	credData, _ := os.ReadFile(paths.Credentials)
	if !contains(string(credData), "beta") {
		t.Error("beta section should still exist in credentials after esc")
	}
}

func TestDeleteConfirm_other_key_cancels(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.deleteTarget = "beta"
	m.Update(key("x"))
	if m.deleteTarget != "" {
		t.Error("deleteTarget should be cleared on any non-y key")
	}
	if !contains(m.message, "cancel") {
		t.Errorf("expected a cancelled message, got %q", m.message)
	}
}

func TestDeleteConfirm_error_shows_message_and_keeps_running(t *testing.T) {
	dir := t.TempDir()
	paths := newDeletePaths(dir)
	// Make Credentials an invalid path: a directory instead of a file, so
	// RemoveProfile's ini.LoadSources call fails.
	if err := os.Mkdir(paths.Credentials, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = paths
	m.deleteTarget = "beta"

	// A reload is attempted even on error, but this fixture breaks reads too
	// (credentials is a directory), so reload fails and yields a nil cmd with
	// its own failure message.
	_, cmd := m.Update(key("y"))
	if cmd != nil {
		t.Error("expected a nil command when the post-error reload cannot read the fixture")
	}
	if m.deleteTarget != "" {
		t.Error("deleteTarget should be cleared even on error")
	}
	if !contains(m.message, "fail") && !contains(m.message, "error") {
		t.Errorf("expected an error/failure message, got %q", m.message)
	}
	// TUI keeps running: further keys are still processed normally.
	_, cmd = m.Update(key("r"))
	if cmd == nil {
		t.Error("TUI should keep processing keys after a delete error")
	}
}

func TestView_footer_advertises_delete(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	if !contains(m.View(), "d delete") {
		t.Error("expected the footer to mention 'd delete'")
	}
}

// AC1 also requires the prompt to display the profile's detected type, not
// just its name.
func TestUpdate_d_shows_delete_confirm_with_type(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1 // beta, TypeManual
	m.Update(key("d"))
	out := m.View()
	if !contains(out, "beta") {
		t.Error("expected the confirm screen to show the profile name")
	}
	if !contains(out, "manual") {
		t.Errorf("expected the confirm screen to show the profile type, got: %s", out)
	}
}

// AC1: "the list keybindings are suspended" — while the confirm prompt is
// pending, navigation keys must not move the cursor; any non-y/Y key cancels
// the pending delete instead of falling through to handleKey.
func TestDeleteConfirm_suspends_list_navigation_keys(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 0
	m.deleteTarget = "beta"

	m.Update(key("down"))

	if m.cursor != 0 {
		t.Errorf("cursor should not move while delete confirm is pending, got %d", m.cursor)
	}
	if m.deleteTarget != "" {
		t.Error("the down key should have cancelled the pending delete, not navigated")
	}
}

// AC2 explicitly allows uppercase Y as well as lowercase y.
func TestDeleteConfirm_uppercase_Y_also_confirms(t *testing.T) {
	dir := t.TempDir()
	paths := newDeletePaths(dir)
	seedManualProfile(t, paths, "beta")
	seedManualProfile(t, paths, "alpha") // survives, so reload's checkAllCmd is non-nil

	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = paths
	m.deleteTarget = "beta"

	_, cmd := m.Update(key("Y"))
	if cmd == nil {
		t.Fatal("expected a reload command after confirming with uppercase Y")
	}
	if !contains(m.message, "deleted") {
		t.Errorf("expected a success message, got %q", m.message)
	}
	if _, ok := profiles.Find(m.profiles, "beta"); ok {
		t.Error("expected beta to be gone from the reloaded profile list")
	}
}

// AC3: cancelling must actually return to the list view (not leave the
// confirm screen rendering).
func TestDeleteConfirm_cancel_returns_to_list_view(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.deleteTarget = "beta"

	m.Update(key("n"))

	out := m.View()
	if contains(out, "Delete profile") {
		t.Error("expected the view to return to the list, not stay on the confirm screen")
	}
	if !contains(out, "d delete") {
		t.Error("expected the normal list footer to render again after cancel")
	}
}

// Edge case: while the filter query is non-empty, every single-rune key
// (including 'd') is appended to the filter text instead of triggering its
// list action — this matches the existing behavior for 'a'/'s'/'t'/'l' and is
// not specific to delete, but it means "d" cannot delete while actively
// composing a filter query; the user must commit the filter (enter/esc) first
// and press 'd' again.
func TestUpdate_d_while_filter_active_is_swallowed_by_filter_text(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("/"))
	m.Update(key("b")) // filter now "b", matches "beta"

	m.Update(key("d")) // should be appended to the filter, not trigger delete

	if m.deleteTarget != "" {
		t.Errorf("expected 'd' to be swallowed by the active filter, got deleteTarget %q", m.deleteTarget)
	}
	// '/' seeds the filter with a leading space (trimmed only on display/
	// commit), so the accumulated text is " bd", not "bd".
	if strings.TrimSpace(m.filter) != "bd" {
		t.Errorf("expected filter to accumulate 'd', got %q", m.filter)
	}
}

// Edge case: the delete and load flows are mutually exclusive and the load
// flow's paste textarea is entered first, so 'd' pressed mid-paste must be
// treated as a literal character typed into the textarea, not a delete
// trigger.
func TestUpdate_d_during_load_paste_is_typed_not_delete(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1 // beta
	m.Update(key("l"))
	if m.loadStep != loadPaste {
		t.Fatalf("expected loadPaste, got %v", m.loadStep)
	}

	m.Update(key("d"))

	if m.deleteTarget != "" {
		t.Errorf("expected 'd' during paste to not start a delete, got deleteTarget %q", m.deleteTarget)
	}
	if m.loadStep != loadPaste {
		t.Error("expected to remain in the paste step")
	}
	if !contains(m.ta.Value(), "d") {
		t.Errorf("expected 'd' to be typed into the textarea, got %q", m.ta.Value())
	}
}

// Edge case: at the load-confirm step, any non-y/Y/enter key — including 'd'
// — is treated as "cancel the load", per handleConfirmKey's default branch.
// It must not leak into starting a delete.
func TestUpdate_d_during_load_confirm_cancels_load_not_delete(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1 // beta
	m.Update(key("l"))
	m.ta.SetValue("export AWS_ACCESS_KEY_ID=ASIACLIP0002\nexport AWS_SECRET_ACCESS_KEY=sec\n")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.loadStep != loadConfirm {
		t.Fatalf("expected loadConfirm, got %v", m.loadStep)
	}

	m.Update(key("d"))

	if m.loadStep != loadNone {
		t.Error("expected 'd' at the load-confirm step to cancel the load")
	}
	if m.deleteTarget != "" {
		t.Errorf("expected no delete to start, got deleteTarget %q", m.deleteTarget)
	}
	if !contains(m.message, "cancel") {
		t.Errorf("expected a cancel message, got %q", m.message)
	}
}

// Edge case: deleting the only (or last-in-filter) selected profile must not
// leave the cursor out of range for the now-shorter filtered list.
func TestDeleteConfirm_y_on_sole_profile_clamps_cursor(t *testing.T) {
	dir := t.TempDir()
	paths := newDeletePaths(dir)
	seedManualProfile(t, paths, "solo")

	m := newModel(runner.NewFake(), []profiles.Profile{{Name: "solo", Type: profiles.TypeManual}}, "")
	m.paths = paths
	m.deleteTarget = "solo"

	// reload() re-reads the list and re-applies the filter synchronously
	// before returning the (deferred) status-check command; with zero
	// profiles left, checkAllCmd's tea.Batch of zero commands collapses to a
	// nil Cmd, so we assert on model state directly rather than on cmd.
	m.Update(key("y"))

	if len(m.profiles) != 0 {
		t.Fatalf("expected the profile list to be empty after deleting the sole profile, got %d", len(m.profiles))
	}
	if len(m.filtered) != 0 {
		t.Fatalf("expected an empty filtered list, got %d", len(m.filtered))
	}
	if m.cursor != 0 {
		t.Errorf("expected cursor clamped to 0, got %d", m.cursor)
	}
	// Must not panic.
	out := m.View()
	if !contains(out, "no profiles match") {
		t.Errorf("expected the empty-list message, got: %s", out)
	}
}

// Edge case: if the in-memory profile list no longer contains the pending
// delete target (e.g. it was removed by a concurrent reload triggered by
// another message), the confirm screen must still render without panicking.
func TestDeleteConfirmView_renders_when_profile_missing_from_list(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.deleteTarget = "ghost-profile-not-in-list"

	out := m.View()

	if !contains(out, "ghost-profile-not-in-list") {
		t.Errorf("expected the confirm screen to still show the target name, got: %s", out)
	}
	if !contains(out, "type:") {
		t.Errorf("expected the type line to still render, got: %s", out)
	}
}

// Risk from the spec ("Removal partially succeeds"): if the credentials write
// succeeds but the config removal fails, doDelete surfaces the error AND still
// reloads, so the list keeps mirroring disk. Here beta survives the reload —
// correctly — because its config section and override are still on disk.
func TestDeleteConfirm_y_config_removal_failure_surfaces_error_and_reloads(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based write denial does not apply to root")
	}
	dir := t.TempDir()
	paths := newDeletePaths(dir)
	seedManualProfile(t, paths, "beta")
	// A read-only config file lets RemoveConfigProfile load the section but
	// fail on save, after RemoveProfile has already succeeded.
	if err := os.Chmod(paths.Config, 0o400); err != nil {
		t.Fatalf("setup chmod: %v", err)
	}

	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = paths
	m.deleteTarget = "beta"

	_, cmd := m.Update(key("y"))

	if !contains(m.message, "error deleting beta") {
		t.Errorf("expected a delete error message, got %q", m.message)
	}
	if m.deleteTarget != "" {
		t.Error("deleteTarget should be cleared even on partial failure")
	}
	if cmd == nil {
		t.Error("expected a reload command even when config removal fails")
	}

	credData, err := os.ReadFile(paths.Credentials)
	if err != nil {
		t.Fatalf("reading credentials fixture: %v", err)
	}
	if contains(string(credData), "beta") {
		t.Error("expected credentials removal to have already happened before the config step failed")
	}
	// The reloaded list mirrors disk: beta is still defined by config+override,
	// so it must remain visible.
	if _, ok := profiles.Find(m.profiles, "beta"); !ok {
		t.Error("expected beta to survive the reload while its config section still exists on disk")
	}
}

// Covers doDelete's third error branch (override write failure) after both the
// credentials and config removals succeeded. This is the case the reload-on-
// error behavior exists for: beta is gone from every file that defined it, so
// the reloaded list must drop it even though the delete reported an error.
func TestDeleteConfirm_y_override_removal_failure_still_reloads_list(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based write denial does not apply to root")
	}
	dir := t.TempDir()
	paths := newDeletePaths(dir)
	// Seed beta WITHOUT an override so the reloaded list has no reason to keep
	// it once credentials and config are gone.
	in := profiles.ManualInput{AccessKeyID: "AKIASEED0002", Secret: "sekret"}
	if err := profiles.AddManual(paths.Credentials, paths.Config, "beta", in); err != nil {
		t.Fatalf("seed AddManual: %v", err)
	}
	// A read-only overrides file (holding an unrelated entry) makes
	// SetOverride load fine but fail on save.
	if err := profiles.SetOverride(paths.Overrides, "other", profiles.Override{Type: profiles.TypeManual}); err != nil {
		t.Fatalf("seed SetOverride: %v", err)
	}
	if err := os.Chmod(paths.Overrides, 0o400); err != nil {
		t.Fatalf("setup chmod: %v", err)
	}

	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.paths = paths
	m.deleteTarget = "beta"

	_, cmd := m.Update(key("y"))

	if !contains(m.message, "error deleting beta") {
		t.Errorf("expected a delete error message, got %q", m.message)
	}
	if cmd == nil {
		t.Error("expected a reload command even when the override write fails")
	}

	credData, _ := os.ReadFile(paths.Credentials)
	if contains(string(credData), "beta") {
		t.Error("expected credentials removal to have already happened before the override step failed")
	}
	configData, _ := os.ReadFile(paths.Config)
	if contains(string(configData), "beta") {
		t.Error("expected config removal to have already happened before the override step failed")
	}
	// The list must not keep showing a profile no file defines anymore.
	if _, ok := profiles.Find(m.profiles, "beta"); ok {
		t.Error("expected the reloaded list to drop beta after its files were removed, despite the override error")
	}
}

// Interleaving edge case: pressing 'a' while a delete confirmation is
// pending does NOT open the type menu. deleteTarget is checked ahead of the
// load flow in Update, so the key is routed to handleDeleteConfirmKey
// instead; since 'a' isn't 'y'/'Y' there, it cancels the pending delete. The
// user is not stuck -- pressing 'a' again from the (now normal) list view
// does open the type menu -- but the first 'a' is "spent" cancelling delete,
// not starting add.
func TestUpdate_a_during_delete_confirm_cancels_delete_not_add(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.cursor = 1 // beta
	m.deleteTarget = "beta"

	m.Update(key("a"))

	if m.deleteTarget != "" {
		t.Errorf("expected 'a' to cancel the pending delete, got deleteTarget %q", m.deleteTarget)
	}
	if m.loadStep != loadNone {
		t.Errorf("expected 'a' during delete confirm to NOT open the type menu, got loadStep %v", m.loadStep)
	}
	if !contains(m.message, "cancel") {
		t.Errorf("expected a cancel message, got %q", m.message)
	}

	// The user is not stuck: a second 'a' (now that delete is cancelled and
	// the list view is active again) does open the type menu.
	m.Update(key("a"))
	if m.loadStep != loadType {
		t.Errorf("expected a second 'a' to open the type menu, got %v", m.loadStep)
	}
}

// The type menu's typeCursor is reset to 0 after a cancel, so a later 'a'
// doesn't reopen the menu pre-scrolled to wherever the user left it.
func TestAddType_typeCursor_resets_after_cancel(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("a"))
	m.Update(key("down"))
	m.Update(key("down"))
	if m.typeCursor == 0 {
		t.Fatal("setup: expected typeCursor to have moved before cancelling")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // cancel

	m.Update(key("a")) // reopen
	if m.typeCursor != 0 {
		t.Errorf("expected typeCursor reset to 0 after cancel + reopen, got %d", m.typeCursor)
	}
}

// The duplicate-name check at the name step runs against the FULL profile
// list, not the currently-filtered/displayed subset -- filtering only
// affects what's shown, so a name hidden by an active filter must still be
// rejected as a duplicate.
func TestAdd_name_rejects_duplicate_against_full_list_not_filtered_view(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	// Filter down to something that does NOT match "beta", so beta is hidden
	// from the visible list but still exists in m.profiles.
	m.filter = "alpha"
	m.applyFilter()
	if len(m.filtered) != 1 {
		t.Fatalf("setup: expected filter to hide beta, got %d filtered rows", len(m.filtered))
	}

	m.loadStep = loadType
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // manual -> name step
	for _, r := range "beta" {
		m.Update(key(string(r)))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.loadStep != loadName {
		t.Error("expected 'beta' to be rejected as a duplicate even though it's hidden by the active filter")
	}
	if !contains(m.message, "ya existe") {
		t.Errorf("expected duplicate message, got %q", m.message)
	}
}

// Profile names containing spaces or path-like slashes round-trip through
// the manual-creation flow (AddManual/profiles.List) without corruption --
// there is no name validation beyond empty/duplicate at the TUI layer, and
// the underlying ini-backed storage tolerates both characters.
func TestAdd_name_with_space_and_slash_round_trips(t *testing.T) {
	for _, name := range []string{"my profile", "team/dev"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			m := newModel(runner.NewFake(), sampleProfiles(), "")
			m.paths = profiles.Paths{
				Credentials: filepath.Join(dir, "credentials"),
				Config:      filepath.Join(dir, "config"),
				Overrides:   filepath.Join(dir, "profiles.ini"),
			}
			m.Update(key("a"))
			m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // manual -> name step
			for _, r := range name {
				m.Update(key(string(r)))
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if m.loadStep != loadPaste {
				t.Fatalf("expected loadPaste for name %q, got %v (message: %q)", name, m.loadStep, m.message)
			}
			m.ta.SetValue("export AWS_ACCESS_KEY_ID=ASIANAME0001\nexport AWS_SECRET_ACCESS_KEY=sec\n")
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
			m.Update(key("y"))

			list, err := profiles.List(m.paths)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			p, ok := profiles.Find(list, name)
			if !ok || p.Type != profiles.TypeManual {
				t.Fatalf("expected %q stored as manual, got %+v (ok=%v)", name, p, ok)
			}
		})
	}
}

// Coverage gap: pasteView (the loadPaste screen) was never rendered directly
// via View() by the other tests, which drive the flow through Update without
// checking the screen text at that step.
func TestView_paste_step_renders_pasteView(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.loadStep = loadPaste
	m.loadProfile = "delta"
	m.message = "some validation hint"
	out := m.View()
	if !contains(out, "delta") {
		t.Errorf("expected the paste view to show the target profile name, got:\n%s", out)
	}
	if !contains(out, "some validation hint") {
		t.Errorf("expected the paste view to render the pending message, got:\n%s", out)
	}
	if !contains(out, "ctrl+d") {
		t.Errorf("expected the paste view footer hint, got:\n%s", out)
	}
}

// Coverage gap: nameView's message branch (validation hints) was only
// asserted against m.message directly, never against the rendered view.
func TestView_name_step_renders_validation_message(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.loadStep = loadName
	m.nameInput = "beta"
	m.message = "ya existe un profile llamado beta"
	out := m.View()
	if !contains(out, "ya existe") {
		t.Errorf("expected the name view to render the validation message, got:\n%s", out)
	}
}

// Coverage gap: confirmView's "temporary" branch (session token present) was
// exercised via doLoad but never asserted on directly; the other direct
// confirmView test only covers the no-token ("no") branch.
func TestView_confirm_step_shows_temporary_when_session_token_present(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.loadStep = loadConfirm
	m.loadProfile = "delta"
	m.loadParsed = creds.Parsed{AccessKeyID: "ASIA0001", SecretAccessKey: "sec", SessionToken: "tok"}
	out := m.confirmView()
	if !contains(out, "temporales") {
		t.Errorf("expected the confirm view to flag temporary credentials when a session token is present, got:\n%s", out)
	}
}

// Coverage gap: 't' (set-type) on an empty/filtered-out list must be a
// no-op, not attempt to launch the CLI wizard against a zero-value profile.
func TestUpdate_t_on_empty_list_is_noop(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.filter = "zzz-no-match"
	m.applyFilter()

	_, cmd := m.Update(key("t"))
	if cmd != nil {
		t.Error("expected no command when set-type has nothing selected")
	}
}

// Coverage gap: Update's reloadMsg branch (both the success and failure
// message formatting) was never exercised directly.
func TestUpdate_reloadMsg_success_and_failure_messages(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(reloadMsg{action: "add", err: nil})
	if !contains(m.message, "add") || !contains(m.message, "reloaded") {
		t.Errorf("expected a success reload message, got %q", m.message)
	}

	m2 := newModel(runner.NewFake(), sampleProfiles(), "")
	m2.Update(reloadMsg{action: "add --type sso", err: os.ErrPermission})
	if !contains(m2.message, "add --type sso") || !contains(m2.message, "failed") {
		t.Errorf("expected a failure reload message, got %q", m2.message)
	}
}

// Coverage gap: while the filter is active, keys that don't map to a single
// rune (like the up/down arrows) fall through handleFilterKey's default
// branch unhandled and must still perform their normal navigation action
// instead of being silently dropped.
func TestHandleFilterKey_arrow_keys_still_navigate_while_filtering(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("/"))
	// No filter text yet, but filter mode is active ("/" seeds a leading
	// space); typing nothing and pressing down should still move the cursor
	// through the full (unfiltered) list rather than being swallowed.
	m.Update(key("down"))
	if m.cursor != 1 {
		t.Errorf("expected down-arrow to navigate while filter is active, cursor got %d want 1", m.cursor)
	}
	if strings.Contains(m.filter, "down") {
		t.Errorf("expected the arrow key to not be appended to the filter text, got %q", m.filter)
	}
}

// Coverage gap: doLoad's error path (profiles.AddManual failing) was never
// exercised -- only the success path is covered by
// TestAdd_name_then_paste_creates_manual_profile.
func TestDoLoad_write_error_surfaces_message_and_does_not_panic(t *testing.T) {
	dir := t.TempDir()
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	// Credentials path points at a directory, so AddManual's ini write fails.
	credPath := filepath.Join(dir, "credentials")
	if err := os.Mkdir(credPath, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	m.paths = profiles.Paths{
		Credentials: credPath,
		Config:      filepath.Join(dir, "config"),
		Overrides:   filepath.Join(dir, "profiles.ini"),
	}
	m.loadStep = loadConfirm
	m.loadProfile = "delta"
	m.loadParsed = creds.Parsed{AccessKeyID: "ASIA0001", SecretAccessKey: "sec"}

	_, cmd := m.Update(key("y"))

	if !contains(m.message, "error") {
		t.Errorf("expected an error message, got %q", m.message)
	}
	if cmd != nil {
		t.Error("expected no reload command when the write itself fails before reaching reload")
	}
	if m.loadStep != loadNone {
		t.Error("expected the load flow to be reset even on a write error")
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

// Multi-byte runes (ñ, á, …) count as one character for the name buffer, and
// backspace removes the whole rune, never leaving a broken UTF-8 tail.
func TestAdd_name_accepts_multibyte_runes_and_backspace_removes_whole_rune(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.loadStep = loadName

	m.Update(key("a"))
	m.Update(key("ñ"))
	if m.nameInput != "añ" {
		t.Fatalf("expected multibyte rune appended, got %q", m.nameInput)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.nameInput != "a" {
		t.Errorf("backspace should remove the whole rune, got %q", m.nameInput)
	}
}

// The filter shares the rune-aware input handling.
func TestFilter_accepts_multibyte_runes(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.Update(key("/"))
	m.Update(key("ñ"))
	if !contains(m.filter, "ñ") {
		t.Errorf("expected multibyte rune in filter, got %q", m.filter)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if contains(m.filter, "ñ") || !utf8.ValidString(m.filter) {
		t.Errorf("backspace should remove the whole rune cleanly, got %q", m.filter)
	}
}

// The digit-shortcut range and the menu hint both follow len(addableTypes()).
func TestAddType_shortcuts_and_hint_follow_type_count(t *testing.T) {
	m := newModel(runner.NewFake(), sampleProfiles(), "")
	m.loadStep = loadType

	n := len(addableTypes())
	if !contains(m.View(), fmt.Sprintf("1-%d", n)) {
		t.Errorf("hint should advertise 1-%d, got: %s", n, m.View())
	}

	// A digit just past the range is ignored (no panic, no selection).
	m.Update(key(fmt.Sprintf("%d", n+1)))
	if m.loadStep != loadType {
		t.Errorf("out-of-range digit should be ignored, loadStep=%v", m.loadStep)
	}
}
