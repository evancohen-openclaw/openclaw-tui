package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
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

	// Build the main layout
	mainView := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.viewport.View(),
		status,
		footer,
	)

	// Add autocomplete popup between main view and input
	autocomplete := m.renderAutocomplete()
	if autocomplete != "" {
		mainView = lipgloss.JoinVertical(lipgloss.Left, mainView, autocomplete, input)
	} else {
		mainView = lipgloss.JoinVertical(lipgloss.Left, mainView, input)
	}

	// Overlay on top if active
	if m.overlayActive {
		mainView = m.renderWithOverlay(mainView)
	}

	v := tea.NewView(mainView)
	v.AltScreen = true
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (m *Model) updateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	headerH := 1
	footerH := 1
	statusH := 1
	inputH := m.input.Height()
	if inputH < 3 {
		inputH = 3
	}

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

	// Only auto-scroll if user is already near the bottom
	atBottom := m.viewport.ScrollPercent() >= 0.98 || m.viewport.TotalLineCount() <= m.viewport.Height()

	content := m.renderMessages()
	m.viewport.SetContent(content)

	if atBottom {
		m.viewport.GotoBottom()
	}
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

	if m.fastMode == "on" {
		parts = append(parts, "fast")
	}

	if m.verboseLevel != "" && m.verboseLevel != "off" {
		parts = append(parts, fmt.Sprintf("verbose %s", m.verboseLevel))
	}

	if m.reasoningLevel != "" && m.reasoningLevel != "off" {
		parts = append(parts, fmt.Sprintf("reason %s", m.reasoningLevel))
	}

	if m.deliver {
		parts = append(parts, "deliver")
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
		content := msg.content
		lines := strings.Split(content, "\n")
		if len(lines) > 8 {
			content = strings.Join(lines[:4], "\n") +
				fmt.Sprintf("\n… (%d lines collapsed) …\n", len(lines)-5) +
				lines[len(lines)-1]
		}
		style := m.theme.UserMsg.Width(w)
		return "\n" + style.Render(content)

	case "assistant":
		// Render final assistant messages through glamour for markdown
		rendered := m.renderMarkdown(msg.content, w)
		style := m.theme.AssistantMsg.Width(w)
		return "\n" + style.Render(rendered)

	case "assistant-stream":
		// Streaming messages stay raw for performance
		style := m.theme.AssistantMsg.Width(w)
		content := msg.content + " ▌"
		return "\n" + style.Render(content)

	case "thinking":
		style := m.theme.Thinking.Width(w - 4)
		return "\n" + style.Render(msg.content)

	case "system":
		content := msg.content
		lines := strings.Split(content, "\n")
		if len(lines) > 6 {
			content = strings.Join(lines[:3], "\n") +
				fmt.Sprintf("\n… (%d lines collapsed) …\n", len(lines)-4) +
				lines[len(lines)-1]
		}
		return " " + m.theme.System.Render(content)

	case "tool-pending":
		style := m.theme.ToolPending.Width(w)
		return " " + style.Render(msg.content)

	case "tool-success":
		content := msg.content
		if !m.toolsExpanded {
			// Collapse to first line only
			if idx := strings.Index(content, "\n"); idx > 0 {
				content = content[:idx] + " …"
			}
		}
		style := m.theme.ToolSuccess.Width(w)
		return " " + style.Render(content)

	case "tool-error":
		style := m.theme.ToolError.Width(w)
		return " " + style.Render(msg.content)

	case "tool":
		style := m.theme.ToolSuccess.Width(w)
		return "\n" + style.Render(msg.content)

	default:
		return " " + msg.content
	}
}

// renderMarkdown renders content as styled markdown using glamour.
func (m Model) renderMarkdown(content string, width int) string {
	if strings.TrimSpace(content) == "" {
		return content
	}

	styleName := "dark"
	if m.theme.Light {
		styleName = "light"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styleName),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}

	rendered, err := r.Render(content)
	if err != nil {
		return content
	}

	return strings.TrimRight(rendered, "\n")
}

// renderAutocomplete renders the autocomplete popup above the input.
func (m Model) renderAutocomplete() string {
	if !m.autocompleteActive || len(m.autocompleteSuggs) == 0 {
		return ""
	}

	var lines []string
	for i, cmd := range m.autocompleteSuggs {
		label := "/" + cmd
		if i == m.autocompleteIndex {
			lines = append(lines, m.theme.AutocompleteItemActive.Render(label))
		} else {
			lines = append(lines, m.theme.AutocompleteItem.Render(label))
		}
	}

	return strings.Join(lines, "\n")
}

// renderWithOverlay renders the overlay picker on top of the main view.
func (m Model) renderWithOverlay(base string) string {
	if len(m.overlayItems) == 0 {
		return base
	}

	// Build overlay content
	title := m.theme.OverlayTitle.Render(fmt.Sprintf("Select %s", m.overlayType))
	hint := m.theme.Dim.Render("↑/↓ navigate • enter select • esc cancel")

	var items []string
	for i, item := range m.overlayItems {
		label := item.title
		if item.description != "" && item.description != item.title {
			label += m.theme.Dim.Render(" — " + item.description)
		}
		if i == m.overlayIndex {
			items = append(items, m.theme.OverlayItemActive.Render("> "+label))
		} else {
			items = append(items, m.theme.OverlayItem.Render("  "+label))
		}
	}

	overlayContent := title + "\n" + hint + "\n\n" + strings.Join(items, "\n")

	overlayWidth := m.width * 2 / 3
	if overlayWidth < 40 {
		overlayWidth = 40
	}
	if overlayWidth > m.width-4 {
		overlayWidth = m.width - 4
	}

	overlay := m.theme.OverlayBorder.Width(overlayWidth).Render(overlayContent)

	// Center the overlay on the base
	return placeOverlay(m.width, m.height, overlay, base)
}

// placeOverlay centers an overlay on top of a base string.
func placeOverlay(width, height int, overlay, base string) string {
	overlayLines := strings.Split(overlay, "\n")
	baseLines := strings.Split(base, "\n")

	// Pad base to fill height
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}

	overlayH := len(overlayLines)
	overlayW := 0
	for _, line := range overlayLines {
		l := lipgloss.Width(line)
		if l > overlayW {
			overlayW = l
		}
	}

	// Calculate centering
	startY := (height - overlayH) / 2
	if startY < 0 {
		startY = 0
	}
	startX := (width - overlayW) / 2
	if startX < 0 {
		startX = 0
	}

	// Merge overlay onto base
	for i, overlayLine := range overlayLines {
		y := startY + i
		if y >= len(baseLines) {
			break
		}

		baseLine := baseLines[y]
		// Pad base line to startX
		baseRunes := []rune(baseLine)
		for len(baseRunes) < startX {
			baseRunes = append(baseRunes, ' ')
		}

		// Replace portion with overlay
		overlayRunes := []rune(overlayLine)
		result := make([]rune, 0, startX+len(overlayRunes))
		result = append(result, baseRunes[:startX]...)
		result = append(result, overlayRunes...)
		baseLines[y] = string(result)
	}

	return strings.Join(baseLines[:height], "\n")
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
