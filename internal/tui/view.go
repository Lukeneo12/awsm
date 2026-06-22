package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Lukeneo12/awsm/internal/profiles"
	"github.com/Lukeneo12/awsm/internal/status"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	promptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)

	activeBadge  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("● active")
	expiredBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("● expired")
	invalidBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("● invalid")
	pendingBadge = dimStyle.Render("… checking")
)

func (m *model) View() string {
	// The load flow takes over the whole screen.
	switch m.loadStep {
	case loadPaste:
		return m.pasteView()
	case loadConfirm:
		return m.confirmView()
	}

	var b strings.Builder

	header := "awsm — AWS credentials"
	if m.checking > 0 {
		header += fmt.Sprintf("  (%d checking)", m.checking)
	}
	b.WriteString(titleStyle.Render(header) + "\n\n")

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("  no profiles match\n"))
	}

	for row, idx := range m.filtered {
		p := m.profiles[idx]
		cursor := "  "
		nameRender := p.Name
		if row == m.cursor {
			cursor = cursorStyle.Render("▸ ")
			nameRender = selectedStyle.Render(p.Name)
		}
		line := fmt.Sprintf("%s%-22s %-6s %-12s %s",
			cursor,
			nameRender,
			string(p.Type),
			dash(p.Region),
			m.badge(p),
		)
		b.WriteString(line + "\n")
	}

	b.WriteString("\n")
	if m.filter != "" {
		b.WriteString(dimStyle.Render("filter: "+strings.TrimSpace(m.filter)+"_") + "\n")
	}
	if m.message != "" {
		b.WriteString(dimStyle.Render(m.message) + "\n")
	}
	b.WriteString(helpStyle.Render(
		"↑/↓ move · enter login · s switch · a add · l load · t set-type · r refresh · / filter · q quit"))
	return b.String()
}

// pasteView is the screen where the user pastes a credentials block.
func (m *model) pasteView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Load credentials → "+m.loadProfile) + "\n\n")
	b.WriteString(dimStyle.Render("Pegá el bloque de credenciales que te dio AWS:") + "\n\n")
	b.WriteString(m.ta.View() + "\n\n")
	if m.message != "" {
		b.WriteString(promptStyle.Render(m.message) + "\n")
	}
	b.WriteString(helpStyle.Render("ctrl+d previsualizar · esc cancelar"))
	return b.String()
}

// confirmView shows what was parsed (never the secret) and asks to confirm.
func (m *model) confirmView() string {
	p := m.loadParsed
	token := "no"
	if p.SessionToken != "" {
		token = "sí (temporales)"
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Confirmar carga → "+m.loadProfile) + "\n\n")
	b.WriteString(fmt.Sprintf("  access key id:   ****%s\n", last4(p.AccessKeyID)))
	b.WriteString("  secret:          " + dimStyle.Render("(oculto)") + "\n")
	b.WriteString(fmt.Sprintf("  session token:   %s\n", token))
	b.WriteString(fmt.Sprintf("  region:          %s\n\n", dash(p.Region)))
	b.WriteString(promptStyle.Render(fmt.Sprintf("¿Cargar estas credenciales en %s? [y/n]", m.loadProfile)) + "\n")
	b.WriteString(helpStyle.Render("y confirmar · n / esc cancelar"))
	return b.String()
}

func (m *model) badge(p profiles.Profile) string {
	st, ok := m.statuses[p.Name]
	if !ok {
		return pendingBadge
	}
	switch st.State {
	case status.StateActive:
		acct := ""
		if st.AccountID != "" {
			acct = dimStyle.Render(" " + st.AccountID)
		}
		return activeBadge + acct
	case status.StateExpired:
		return expiredBadge
	case status.StateInvalid:
		return invalidBadge
	default:
		return pendingBadge
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// execCommand builds the *exec.Cmd handed to tea.ExecProcess for interactive
// logins (kept here so model.go stays free of os/exec).
func execCommand(name string, args []string) *exec.Cmd {
	return exec.Command(name, args...)
}
