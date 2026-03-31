package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("loading...")
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	status := m.renderStatus()
	input := m.input.View()

	v := tea.NewView(lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.viewport.View(),
		status,
		footer,
		input,
	))
	v.AltScreen = true
	return v
}

func (m *Model) updateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	headerH := 1
	footerH := 1
	statusH := 1
	inputH := 3

	vpHeight := m.height - headerH - footerH - statusH - inputH
	if vpHeight < 1 {
		vpHeight = 1
	}

	if !m.ready {
		m.viewport = viewport.New(
			viewport.WithWidth(m.width),
			viewport.WithHeight(vpHeight),
		)
		m.viewport.SetContent(m.renderMessages())
		m.ready = true
	} else {
		m.viewport.SetWidth(m.width)
		m.viewport.SetHeight(vpHeight)
	}

	m.input.SetWidth(m.width - 2)
}

func (m *Model) updateViewport() {
	if !m.ready {
		return
	}
	content := m.renderMessages()
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m Model) renderHeader() string {
	url := m.cfg.URL
	agent := m.currentAgentID
	if agent == "" {
		agent = "?"
	}
	session := m.formatSessionKey()
	if session == "" {
		session = "?"
	}

	text := fmt.Sprintf("openclaw tui — %s — agent %s — session %s", url, agent, session)
	return m.theme.Header.Width(m.width).Render(text)
}

func (m Model) renderFooter() string {
	parts := []string{}

	if m.currentAgentID != "" {
		parts = append(parts, fmt.Sprintf("agent %s", m.currentAgentID))
	}

	parts = append(parts, fmt.Sprintf("session %s", m.formatSessionKey()))

	if m.model != "" {
		modelLabel := m.model
		if m.modelProvider != "" {
			modelLabel = m.modelProvider + "/" + m.model
		}
		parts = append(parts, modelLabel)
	}

	if m.thinkingLevel != "" && m.thinkingLevel != "off" {
		parts = append(parts, fmt.Sprintf("think %s", m.thinkingLevel))
	}

	parts = append(parts, m.formatTokens())

	text := strings.Join(parts, " │ ")
	return m.theme.Footer.Width(m.width).Render(text)
}

func (m Model) renderStatus() string {
	isBusy := m.activityStatus == actSending ||
		m.activityStatus == actWaiting ||
		m.activityStatus == actStreaming ||
		m.activityStatus == actRunning

	if isBusy {
		elapsed := ""
		if m.runStartedAt != nil {
			d := time.Since(*m.runStartedAt)
			if d < time.Minute {
				elapsed = fmt.Sprintf("%ds", int(d.Seconds()))
			} else {
				elapsed = fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
			}
		}

		text := fmt.Sprintf("%s %s", m.spinner.View(), m.activityStatus)
		if elapsed != "" {
			text += fmt.Sprintf(" • %s", elapsed)
		}
		text += fmt.Sprintf(" │ %s", m.connStatus)
		return m.theme.Status.Width(m.width).Render(text)
	}

	text := fmt.Sprintf("%s │ %s", m.connStatus, m.activityStatus)
	return m.theme.Status.Width(m.width).Render(text)
}

func (m Model) renderMessages() string {
	if len(m.messages) == 0 {
		return m.theme.Dim.Render("  No messages yet. Type a message to start.")
	}

	var sb strings.Builder
	for _, msg := range m.messages {
		sb.WriteString(m.renderMessage(msg))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m Model) renderMessage(msg chatMessage) string {
	w := m.width - 4 // padding
	if w < 10 {
		w = 10
	}

	switch msg.role {
	case "user":
		style := m.theme.UserMsg.Width(w)
		return "\n" + style.Render(msg.content)

	case "assistant", "assistant-stream":
		style := m.theme.AssistantMsg.Width(w)
		content := msg.content
		if msg.role == "assistant-stream" {
			content += " ▌" // cursor indicator for streaming
		}
		return "\n" + style.Render(content)

	case "system":
		return " " + m.theme.System.Render(msg.content)

	case "tool":
		style := m.theme.ToolSuccess.Width(w)
		return "\n" + style.Render(msg.content)

	default:
		return " " + msg.content
	}
}

func (m Model) formatTokens() string {
	if m.totalTokens == nil && m.contextTokens == nil {
		return "tokens ?"
	}

	total := "?"
	if m.totalTokens != nil {
		total = formatTokenCount(*m.totalTokens)
	}

	if m.contextTokens == nil {
		return fmt.Sprintf("tokens %s", total)
	}

	ctx := formatTokenCount(*m.contextTokens)
	pct := ""
	if m.totalTokens != nil && *m.contextTokens > 0 {
		p := *m.totalTokens * 100 / *m.contextTokens
		pct = fmt.Sprintf(" (%d%%)", p)
	}

	return fmt.Sprintf("tokens %s/%s%s", total, ctx, pct)
}

func formatTokenCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}
