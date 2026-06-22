// Package tui renders the interactive profile manager with Bubble Tea.
//
// Switching the active profile cannot be done from a child process, so the TUI
// writes the chosen profile name to a "switch file" handed in by the shell
// wrapper; the wrapper then performs the actual `awsm switch`. Logins run via
// tea.ExecProcess, which releases the terminal to the interactive `aws`/
// `saml2aws` command and resumes the TUI afterwards.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Lukeneo12/awsm/internal/auth"
	"github.com/Lukeneo12/awsm/internal/creds"
	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/runner"
	"github.com/Lukeneo12/awsm/internal/status"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbletea"
)

// loadStep tracks the multi-step "load credentials" flow.
type loadStep int

const (
	loadNone    loadStep = iota // not loading
	loadPaste                   // textarea open, user pasting
	loadConfirm                 // parsed preview shown, awaiting y/n
)

// Run launches the TUI. switchFile, when non-empty, is the path the wrapper
// reads after exit to apply the chosen profile. paths lets the TUI reload the
// profile list after an add/set-type runs.
func Run(r runner.CommandRunner, paths profiles.Paths, list []profiles.Profile, switchFile string) error {
	m := newModel(r, list, switchFile)
	m.paths = paths
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type model struct {
	runner     runner.CommandRunner
	checker    *status.Checker
	authn      *auth.Authenticator
	paths      profiles.Paths
	profiles   []profiles.Profile
	statuses   map[string]status.Status
	switchFile string

	cursor   int
	filter   string
	filtered []int // indices into profiles matching filter
	message  string
	checking int // outstanding status checks

	// load-credentials flow
	loadStep    loadStep
	ta          textarea.Model
	loadProfile string       // target profile for the load
	loadParsed  creds.Parsed // parsed creds awaiting confirmation
}

func newModel(r runner.CommandRunner, list []profiles.Profile, switchFile string) *model {
	ta := textarea.New()
	ta.Placeholder = "pegá acá el bloque de credenciales (export.../[perfil].../$env:...)"
	ta.ShowLineNumbers = false
	ta.SetWidth(72)
	ta.SetHeight(6)

	m := &model{
		runner:     r,
		checker:    status.NewChecker(r),
		authn:      auth.New(r),
		profiles:   list,
		statuses:   map[string]status.Status{},
		switchFile: switchFile,
		ta:         ta,
	}
	m.applyFilter()
	return m
}

// --- messages ---

type statusMsg status.Status
type loginDoneMsg struct {
	profile string
	err     error
}
type reloadMsg struct {
	action string
	err    error
}

func (m *model) Init() tea.Cmd {
	return m.checkAllCmd()
}

// checkAllCmd fires one status check per profile so rows update as each lands.
func (m *model) checkAllCmd() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.profiles))
	m.checking = len(m.profiles)
	for _, p := range m.profiles {
		cmds = append(cmds, m.checkCmd(p))
	}
	return tea.Batch(cmds...)
}

func (m *model) checkCmd(p profiles.Profile) tea.Cmd {
	return func() tea.Msg {
		return statusMsg(m.checker.Check(context.Background(), p))
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.statuses[msg.Profile] = status.Status(msg)
		if m.checking > 0 {
			m.checking--
		}
		return m, nil

	case loginDoneMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("login %s failed: %v", msg.profile, msg.err)
		} else {
			m.message = fmt.Sprintf("login %s done — refreshing status", msg.profile)
		}
		if p, ok := profiles.Find(m.profiles, msg.profile); ok {
			m.checking++
			return m, m.checkCmd(p)
		}
		return m, nil

	case reloadMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("%s failed: %v", msg.action, msg.err)
		} else {
			m.message = msg.action + " done — reloaded"
		}
		return m, m.reload()

	case tea.KeyMsg:
		// The load flow captures input while active.
		switch m.loadStep {
		case loadPaste:
			return m.handlePasteKey(msg)
		case loadConfirm:
			return m.handleConfirmKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

// handlePasteKey routes keys while the paste textarea is open: esc cancels,
// ctrl+d/ctrl+s submits for preview, everything else (incl. paste) goes to the
// textarea.
func (m *model) handlePasteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.endLoad()
		m.message = "carga cancelada"
		return m, nil
	case "ctrl+d", "ctrl+s":
		parsed, err := creds.Parse(m.ta.Value())
		if err != nil {
			m.message = "no encontré credenciales válidas en lo pegado — revisá y reintentá (esc cancela)"
			return m, nil
		}
		m.loadParsed = parsed
		m.loadStep = loadConfirm
		m.ta.Blur()
		m.message = ""
		return m, nil
	default:
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		return m, cmd
	}
}

// handleConfirmKey handles the y/n on the parsed-credentials preview.
func (m *model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		return m.doLoad()
	default:
		m.endLoad()
		m.message = "carga cancelada"
		return m, nil
	}
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filter != "" || filterMode(msg) {
		if cmd, handled := m.handleFilterKey(msg); handled {
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "r":
		m.message = "refreshing status..."
		return m, m.checkAllCmd()
	case "s":
		return m.doSwitch()
	case "enter":
		return m.doLogin()
	case "a":
		return m.runSelf("add")
	case "t":
		return m.runSelf("set-type")
	case "l":
		p, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.loadProfile = p.Name
		m.loadStep = loadPaste
		m.ta.Reset()
		m.ta.Focus()
		m.message = ""
		return m, textarea.Blink
	case "/":
		m.filter = " " // enter filter mode; trimmed on display
		m.applyFilter()
	}
	return m, nil
}

// filterMode reports whether we should treat the key as filter input.
func filterMode(msg tea.KeyMsg) bool {
	return false // filter is toggled explicitly with '/'
}

func (m *model) handleFilterKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		m.filter = ""
		m.applyFilter()
		return nil, true
	case "enter":
		// keep filter, drop into navigation
		m.filter = strings.TrimSpace(m.filter)
		if m.filter == "" {
			m.applyFilter()
		}
		return nil, true
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
		}
		m.applyFilter()
		return nil, true
	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
			m.applyFilter()
			return nil, true
		}
	}
	return nil, false
}

func (m *model) applyFilter() {
	q := strings.TrimSpace(strings.ToLower(m.filter))
	m.filtered = m.filtered[:0]
	for i, p := range m.profiles {
		if q == "" || strings.Contains(strings.ToLower(p.Name), q) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m *model) move(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor = (m.cursor + delta + len(m.filtered)) % len(m.filtered)
}

func (m *model) selected() (profiles.Profile, bool) {
	if len(m.filtered) == 0 {
		return profiles.Profile{}, false
	}
	return m.profiles[m.filtered[m.cursor]], true
}

func (m *model) doSwitch() (tea.Model, tea.Cmd) {
	p, ok := m.selected()
	if !ok {
		return m, nil
	}
	if m.switchFile == "" {
		m.message = "switch needs the shell wrapper — run: eval \"$(awsm shell-init zsh)\""
		return m, nil
	}
	if err := os.WriteFile(m.switchFile, []byte(p.Name), 0o600); err != nil {
		m.message = "could not write switch file: " + err.Error()
		return m, nil
	}
	return m, tea.Quit
}

func (m *model) doLogin() (tea.Model, tea.Cmd) {
	p, ok := m.selected()
	if !ok {
		return m, nil
	}
	// Resolve assume-role to its source first, so we run the source's login
	// command (the role itself has no interactive login).
	target := p
	if p.Type == profiles.TypeRole && p.SourceProfile != "" {
		if src, ok := profiles.Find(m.profiles, p.SourceProfile); ok {
			target = src
		} else {
			m.message = p.Name + ": source profile " + p.SourceProfile + " not found"
			return m, nil
		}
	}
	plan := auth.PlanFor(target)
	if plan.NoOp {
		m.message = p.Name + ": " + plan.Note
		return m, nil
	}

	m.message = "logging in " + target.Name + "..."
	c := execCommand(plan.Binary, plan.Args)
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return loginDoneMsg{profile: target.Name, err: err}
	})
}

// runSelf suspends the TUI and runs this awsm binary's subcommand interactively
// (the add / set-type wizard), then reloads on return. set-type targets the
// selected profile.
func (m *model) runSelf(sub string) (tea.Model, tea.Cmd) {
	self, err := os.Executable()
	if err != nil {
		m.message = "cannot locate awsm binary: " + err.Error()
		return m, nil
	}
	args := []string{sub}
	if sub == "set-type" {
		p, ok := m.selected()
		if !ok {
			return m, nil
		}
		args = append(args, p.Name)
	}
	c := execCommand(self, args)
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return reloadMsg{action: sub, err: err}
	})
}

// doLoad stores the previously-parsed credentials into the target profile.
func (m *model) doLoad() (tea.Model, tea.Cmd) {
	profile := m.loadProfile
	parsed := m.loadParsed
	in := profiles.ManualInput{
		AccessKeyID:  parsed.AccessKeyID,
		Secret:       parsed.SecretAccessKey,
		SessionToken: parsed.SessionToken,
		Region:       parsed.Region,
	}
	m.endLoad()
	if err := profiles.AddManual(m.paths.Credentials, m.paths.Config, profile, in); err != nil {
		m.message = "error guardando: " + err.Error()
		return m, nil
	}
	_ = profiles.SetOverride(m.paths.Overrides, profile, profiles.Override{Type: profiles.TypeManual})
	m.message = "✓ credenciales cargadas en " + profile + " (key ****" + last4(parsed.AccessKeyID) + ")"
	return m, m.reload()
}

// endLoad resets the load flow back to the normal list view.
func (m *model) endLoad() {
	m.loadStep = loadNone
	m.ta.Blur()
	m.ta.Reset()
	m.loadProfile = ""
	m.loadParsed = creds.Parsed{}
}

func last4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}

// reload re-reads the profile list from disk and re-runs status checks.
func (m *model) reload() tea.Cmd {
	list, err := profiles.List(m.paths)
	if err != nil {
		m.message = "reload failed: " + err.Error()
		return nil
	}
	m.profiles = list
	m.statuses = map[string]status.Status{}
	m.applyFilter()
	return m.checkAllCmd()
}
