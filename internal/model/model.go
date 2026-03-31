package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"

	"github.com/evancohen/openclaw-tui/internal/config"
	"github.com/evancohen/openclaw-tui/internal/gateway"
	"github.com/evancohen/openclaw-tui/internal/stream"
	"github.com/evancohen/openclaw-tui/internal/theme"
)

// Activity states
const (
	actIdle       = "idle"
	actSending    = "sending"
	actWaiting    = "waiting"
	actStreaming   = "streaming"
	actRunning    = "running"
	actAborted    = "aborted"
	actError      = "error"
)

// Model is the root Bubble Tea model.
type Model struct {
	cfg    config.Config
	theme  theme.Theme
	client *gateway.ChatClient

	// UI components
	viewport viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	// Layout
	width, height int
	ready         bool

	// Gateway state
	connected      bool
	connStatus     string
	activityStatus string
	helloOk        *gateway.HelloOk

	// Session state
	currentAgentID    string
	currentSessionKey string
	agents            []gateway.AgentEntry
	agentDefault      string
	sessionMainKey    string

	// Session info (from list/patch)
	model         string
	modelProvider string
	thinkingLevel string
	totalTokens   *int
	contextTokens *int

	// Chat state
	messages     []chatMessage
	activeRunID  string
	assembler    *stream.Assembler
	localRunIDs  map[string]bool
	runStartedAt *time.Time

	// Event channel
	eventCh chan tea.Msg

	// Ctrl+C state
	lastCtrlC time.Time
}

type chatMessage struct {
	role    string // "user", "assistant", "system", "tool"
	content string
}

// tickMsg drives periodic updates (elapsed timer, etc.)
type tickMsg time.Time

func doTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// New creates the root model.
func New(cfg config.Config) Model {
	th := theme.New()

	ta := textarea.New()
	ta.Placeholder = "Type a message... (/ for commands)"
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#F6C453"))

	eventCh := make(chan tea.Msg, 100)

	m := Model{
		cfg:            cfg,
		theme:          th,
		input:          ta,
		spinner:        sp,
		connStatus:     "connecting",
		activityStatus: actIdle,
		assembler:      stream.New(),
		localRunIDs:    make(map[string]bool),
		eventCh:        eventCh,
	}

	// Create gateway client
	m.client = gateway.NewChatClient(cfg.URL, cfg.Token, cfg.Password, cfg.Version)

	// Wire up event callbacks
	m.client.OnEvent = func(event string, payload json.RawMessage, seq *int) {
		switch event {
		case "chat":
			var evt gateway.ChatEventPayload
			if err := json.Unmarshal(payload, &evt); err == nil {
				eventCh <- gateway.ChatEventMsg{ChatEventPayload: evt}
			}
		case "agent":
			var evt gateway.AgentEventPayload
			if err := json.Unmarshal(payload, &evt); err == nil {
				eventCh <- gateway.AgentEventMsg{AgentEventPayload: evt}
			}
		}
	}

	m.client.OnConnected = func() {
		eventCh <- gateway.ConnectedMsg{}
	}

	m.client.OnDisconnected = func(reason string) {
		eventCh <- gateway.DisconnectedMsg{Reason: reason}
	}

	m.client.OnHelloOk = func(hello *gateway.HelloOk) {
		eventCh <- gateway.ConnectedMsg{Hello: hello}
	}

	return m
}

func (m Model) Init() tea.Cmd {
	// Start the gateway client in background
	go m.client.Start()

	return tea.Batch(
		m.spinner.Tick,
		waitForEvent(m.eventCh),
		doTick(),
	)
}

func waitForEvent(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case tickMsg:
		cmds = append(cmds, doTick())
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case gateway.ConnectedMsg:
		m.connected = true
		m.connStatus = "connected"
		m.helloOk = msg.Hello
		m.addSystem("connected to gateway")

		// Fetch agents, then load history
		cmds = append(cmds, m.fetchAgents())
		cmds = append(cmds, waitForEvent(m.eventCh))
		return m, tea.Batch(cmds...)

	case gateway.DisconnectedMsg:
		m.connected = false
		m.connStatus = fmt.Sprintf("disconnected: %s", msg.Reason)
		m.addSystem(m.connStatus)
		cmds = append(cmds, waitForEvent(m.eventCh))
		return m, tea.Batch(cmds...)

	case gateway.ChatEventMsg:
		m.handleChatEvent(msg.ChatEventPayload)
		cmds = append(cmds, waitForEvent(m.eventCh))
		return m, tea.Batch(cmds...)

	case gateway.AgentEventMsg:
		m.handleAgentEvent(msg.AgentEventPayload)
		cmds = append(cmds, waitForEvent(m.eventCh))
		return m, tea.Batch(cmds...)

	case gateway.AgentsListMsg:
		if msg.Err != nil {
			m.addSystem(fmt.Sprintf("agents list failed: %v", msg.Err))
		} else {
			m.applyAgents(msg.Result)
			// Now load history
			cmds = append(cmds, m.loadHistory())
		}
		return m, tea.Batch(cmds...)

	case gateway.HistoryLoadedMsg:
		if msg.Err != nil {
			m.addSystem(fmt.Sprintf("history failed: %v", msg.Err))
		} else {
			m.applyHistory(msg.Result)
		}
		cmds = append(cmds, m.refreshSessionInfo())
		return m, tea.Batch(cmds...)

	case gateway.SessionsListMsg:
		if msg.Err == nil && msg.Result != nil {
			m.applySessionInfo(msg.Result)
		}
		return m, tea.Batch(cmds...)

	case gateway.ChatSentMsg:
		if msg.Err != nil {
			m.addSystem(fmt.Sprintf("send failed: %v", msg.Err))
			m.activityStatus = actError
			m.activeRunID = ""
		}
		return m, tea.Batch(cmds...)

	case gateway.StatusResultMsg:
		if msg.Err != nil {
			m.addSystem(fmt.Sprintf("status failed: %v", msg.Err))
		} else {
			m.addSystem(formatStatus(msg.Payload))
		}
		return m, tea.Batch(cmds...)

	case gateway.SessionPatchedMsg:
		if msg.Err != nil {
			m.addSystem(fmt.Sprintf("patch failed: %v", msg.Err))
		}
		cmds = append(cmds, m.refreshSessionInfo())
		return m, tea.Batch(cmds...)

	case gateway.SessionResetMsg:
		if msg.Err != nil {
			m.addSystem(fmt.Sprintf("reset failed: %v", msg.Err))
		} else {
			m.addSystem(fmt.Sprintf("session %s reset", m.currentSessionKey))
			cmds = append(cmds, m.loadHistory())
		}
		return m, tea.Batch(cmds...)

	case gateway.ModelsListMsg:
		if msg.Err != nil {
			m.addSystem(fmt.Sprintf("models list failed: %v", msg.Err))
		} else if msg.Result != nil {
			var lines []string
			lines = append(lines, "Available models:")
			for _, model := range msg.Result.Models {
				name := model.Name
				if name == "" || name == model.ID {
					name = ""
				} else {
					name = " (" + name + ")"
				}
				lines = append(lines, fmt.Sprintf("  %s/%s%s", model.Provider, model.ID, name))
			}
			m.addSystem(strings.Join(lines, "\n"))
		}
		return m, tea.Batch(cmds...)
	}

	// Update sub-components
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	m.viewport, cmd = m.viewport.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.handleCtrlC()
	case "ctrl+d":
		m.client.Stop()
		return m, tea.Quit
	case "enter":
		if m.input.Focused() {
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			return m.handleSubmit(text)
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) handleCtrlC() (tea.Model, tea.Cmd) {
	now := time.Now()

	// If there's input, clear it
	if strings.TrimSpace(m.input.Value()) != "" {
		m.input.Reset()
		m.activityStatus = "cleared; ctrl+c again to exit"
		m.lastCtrlC = now
		return m, nil
	}

	// Double ctrl+c = exit
	if now.Sub(m.lastCtrlC) < time.Second {
		m.client.Stop()
		return m, tea.Quit
	}

	m.lastCtrlC = now
	m.activityStatus = "press ctrl+c again to exit"
	return m, nil
}

func (m *Model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	// Slash commands
	if strings.HasPrefix(text, "/") {
		return m.handleCommand(text)
	}

	// Regular message
	if !m.connected {
		m.addSystem("not connected — message not sent")
		return m, nil
	}

	m.addUser(text)
	runID := uuid.New().String()
	m.activeRunID = runID
	m.localRunIDs[runID] = true
	m.activityStatus = actSending
	now := time.Now()
	m.runStartedAt = &now

	return m, m.sendChat(text, runID)
}

func (m *Model) handleCommand(raw string) (tea.Model, tea.Cmd) {
	parts := strings.SplitN(strings.TrimPrefix(raw, "/"), " ", 2)
	name := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	switch name {
	case "help":
		m.addSystem(helpText())
		return m, nil

	case "exit", "quit":
		m.client.Stop()
		return m, tea.Quit

	case "new":
		newKey := fmt.Sprintf("tui-%s", uuid.New().String())
		m.currentSessionKey = m.resolveSessionKey(newKey)
		m.activeRunID = ""
		m.totalTokens = nil
		m.assembler.Reset()
		m.messages = nil
		m.addSystem(fmt.Sprintf("new session: %s", m.formatSessionKey()))
		return m, m.loadHistory()

	case "reset":
		m.totalTokens = nil
		return m, m.resetSession()

	case "abort":
		if m.activeRunID == "" {
			m.addSystem("no active run")
			return m, nil
		}
		return m, m.abortChat()

	case "status":
		return m, m.fetchStatus()

	case "model":
		if args == "" {
			m.addSystem(fmt.Sprintf("current model: %s/%s", m.modelProvider, m.model))
			m.addSystem("usage: /model <provider/model> (or /models to list)")
			return m, nil
		}
		return m, m.patchModel(args)

	case "models":
		return m, m.fetchModels()

	case "agent":
		if args == "" {
			m.addSystem(fmt.Sprintf("current agent: %s", m.currentAgentID))
			m.addSystem("usage: /agent <id> (or /agents to list)")
			return m, nil
		}
		m.currentAgentID = args
		m.currentSessionKey = m.resolveSessionKey("")
		m.messages = nil
		m.assembler.Reset()
		return m, m.loadHistory()

	case "agents":
		return m, m.fetchAgentsList()

	case "session":
		if args == "" {
			m.addSystem(fmt.Sprintf("current session: %s", m.formatSessionKey()))
			return m, nil
		}
		m.currentSessionKey = m.resolveSessionKey(args)
		m.messages = nil
		m.assembler.Reset()
		m.activeRunID = ""
		return m, m.loadHistory()

	case "sessions":
		return m, m.fetchSessionsList()

	case "think":
		if args == "" {
			m.addSystem(fmt.Sprintf("thinking: %s", m.thinkingLevel))
			return m, nil
		}
		return m, m.patchThinking(args)

	default:
		// Unknown /command → send as chat message (gateway-registered commands)
		if !m.connected {
			m.addSystem("not connected")
			return m, nil
		}
		m.addUser(raw)
		runID := uuid.New().String()
		m.activeRunID = runID
		m.localRunIDs[runID] = true
		m.activityStatus = actSending
		now := time.Now()
		m.runStartedAt = &now
		return m, m.sendChat(raw, runID)
	}
}

// --- Chat event handling ---

func (m *Model) handleChatEvent(evt gateway.ChatEventPayload) {
	if !strings.EqualFold(evt.SessionKey, m.currentSessionKey) {
		return
	}

	if m.activeRunID == "" {
		m.activeRunID = evt.RunID
	}

	switch evt.State {
	case "delta":
		text := m.assembler.IngestDelta(evt.RunID, evt.Message)
		if text != "" {
			m.updateAssistant(evt.RunID, text)
			m.activityStatus = actStreaming
		}

	case "final":
		text := m.assembler.Finalize(evt.RunID, evt.Message, evt.ErrorMessage)
		m.finalizeAssistant(evt.RunID, text)
		if m.activeRunID == evt.RunID {
			m.activeRunID = ""
			m.activityStatus = actIdle
			m.runStartedAt = nil
		}
		delete(m.localRunIDs, evt.RunID)

	case "aborted":
		m.assembler.Drop(evt.RunID)
		m.addSystem("run aborted")
		if m.activeRunID == evt.RunID {
			m.activeRunID = ""
			m.activityStatus = actAborted
			m.runStartedAt = nil
		}

	case "error":
		m.assembler.Drop(evt.RunID)
		errMsg := evt.ErrorMessage
		if errMsg == "" {
			errMsg = "unknown error"
		}
		m.addSystem(fmt.Sprintf("run error: %s", errMsg))
		if m.activeRunID == evt.RunID {
			m.activeRunID = ""
			m.activityStatus = actError
			m.runStartedAt = nil
		}
	}

	m.updateViewport()
}

func (m *Model) handleAgentEvent(evt gateway.AgentEventPayload) {
	// For now, just track lifecycle for status display
	if evt.Stream == "lifecycle" {
		var data struct {
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal(evt.Data, &data); err == nil {
			switch data.Phase {
			case "start":
				m.activityStatus = actRunning
			case "end":
				// Don't override if we're streaming
				if m.activityStatus == actRunning {
					m.activityStatus = actIdle
				}
			case "error":
				m.activityStatus = actError
			}
		}
	}
}

// --- Message management ---

func (m *Model) addSystem(text string) {
	m.messages = append(m.messages, chatMessage{role: "system", content: text})
	m.updateViewport()
}

func (m *Model) addUser(text string) {
	m.messages = append(m.messages, chatMessage{role: "user", content: text})
	m.updateViewport()
}

func (m *Model) updateAssistant(runID string, text string) {
	// Find or create the streaming assistant message
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].role == "assistant-stream" {
			m.messages[i].content = text
			m.updateViewport()
			return
		}
	}
	// Not found, create new
	m.messages = append(m.messages, chatMessage{role: "assistant-stream", content: text})
	m.updateViewport()
}

func (m *Model) finalizeAssistant(runID string, text string) {
	// Replace streaming message with final, or add new
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].role == "assistant-stream" {
			m.messages[i].role = "assistant"
			m.messages[i].content = text
			m.updateViewport()
			return
		}
	}
	m.messages = append(m.messages, chatMessage{role: "assistant", content: text})
	m.updateViewport()
}

// --- Session management ---

func (m *Model) applyAgents(result *gateway.AgentsListResult) {
	m.agents = result.Agents
	m.agentDefault = result.DefaultID
	m.sessionMainKey = result.MainKey

	if m.currentAgentID == "" {
		m.currentAgentID = result.DefaultID
	}

	// Resolve initial session key
	if m.currentSessionKey == "" {
		m.currentSessionKey = m.resolveSessionKey(m.cfg.Session)
	}
}

func (m *Model) applyHistory(result *gateway.ChatHistoryResult) {
	m.messages = nil
	m.assembler.Reset()

	if result.ThinkingLevel != "" {
		m.thinkingLevel = result.ThinkingLevel
	}

	m.addSystem(fmt.Sprintf("session %s", m.formatSessionKey()))

	for _, rawMsg := range result.Messages {
		var record gateway.MessageRecord
		if err := json.Unmarshal(rawMsg, &record); err != nil {
			continue
		}

		if record.Command {
			text := extractText(record)
			if text != "" {
				m.messages = append(m.messages, chatMessage{role: "system", content: text})
			}
			continue
		}

		switch record.Role {
		case "user":
			text := extractText(record)
			if text != "" {
				m.messages = append(m.messages, chatMessage{role: "user", content: text})
			}
		case "assistant":
			text := extractText(record)
			if text != "" {
				m.messages = append(m.messages, chatMessage{role: "assistant", content: text})
			}
		}
	}

	m.updateViewport()
}

func (m *Model) applySessionInfo(result *gateway.SessionsListResult) {
	// Find current session in list
	for _, s := range result.Sessions {
		if strings.EqualFold(s.Key, m.currentSessionKey) {
			if s.Model != "" {
				m.model = s.Model
			}
			if s.ModelProvider != "" {
				m.modelProvider = s.ModelProvider
			}
			if s.ThinkingLevel != "" {
				m.thinkingLevel = s.ThinkingLevel
			}
			m.totalTokens = s.TotalTokens
			m.contextTokens = s.ContextTokens
			break
		}
	}
	if result.Defaults != nil {
		if m.model == "" {
			m.model = result.Defaults.Model
		}
		if m.modelProvider == "" {
			m.modelProvider = result.Defaults.ModelProvider
		}
		if m.contextTokens == nil {
			m.contextTokens = result.Defaults.ContextTokens
		}
	}
}

func (m *Model) resolveSessionKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		if m.currentAgentID == "" {
			return "global"
		}
		mainKey := m.sessionMainKey
		if mainKey == "" {
			mainKey = "main"
		}
		return fmt.Sprintf("agent:%s:%s", m.currentAgentID, mainKey)
	}
	if trimmed == "global" || trimmed == "unknown" {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "agent:") {
		return strings.ToLower(trimmed)
	}
	return fmt.Sprintf("agent:%s:%s", m.currentAgentID, strings.ToLower(trimmed))
}

func (m *Model) formatSessionKey() string {
	key := m.currentSessionKey
	if strings.HasPrefix(key, "agent:") {
		parts := strings.SplitN(key, ":", 3)
		if len(parts) == 3 {
			return parts[2]
		}
	}
	return key
}

// --- RPC command wrappers (return tea.Cmd) ---

func (m *Model) sendChat(text, runID string) tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		err := client.SendChat(sessionKey, text, runID)
		return gateway.ChatSentMsg{RunID: runID, Err: err}
	}
}

func (m *Model) abortChat() tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	runID := m.activeRunID
	return func() tea.Msg {
		err := client.AbortChat(sessionKey, runID)
		if err != nil {
			return gateway.GatewayErrorMsg{Err: err}
		}
		return nil
	}
}

func (m *Model) loadHistory() tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		result, err := client.LoadHistory(sessionKey, 200)
		return gateway.HistoryLoadedMsg{Result: result, Err: err}
	}
}

func (m *Model) fetchAgents() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		result, err := client.ListAgents()
		return gateway.AgentsListMsg{Result: result, Err: err}
	}
}

func (m *Model) fetchAgentsList() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		result, err := client.ListAgents()
		if err != nil {
			return gateway.AgentsListMsg{Err: err}
		}
		// Format as system message for now (overlay picker in Phase 2)
		var lines []string
		lines = append(lines, "Available agents:")
		for _, a := range result.Agents {
			name := a.Name
			if name == "" {
				name = a.ID
			}
			def := ""
			if a.ID == result.DefaultID {
				def = " (default)"
			}
			lines = append(lines, fmt.Sprintf("  %s — %s%s", a.ID, name, def))
		}
		return gateway.StatusResultMsg{Payload: json.RawMessage(`"` + strings.Join(lines, "\n") + `"`)}
	}
}

func (m *Model) fetchSessionsList() tea.Cmd {
	client := m.client
	agentID := m.currentAgentID
	return func() tea.Msg {
		result, err := client.ListSessions(gateway.SessionsListParams{
			IncludeDerivedTitles: true,
			IncludeLastMessage:   true,
			AgentID:              agentID,
		})
		if err != nil {
			return gateway.StatusResultMsg{Err: err}
		}
		var lines []string
		lines = append(lines, "Sessions:")
		for _, s := range result.Sessions {
			title := s.DerivedTitle
			if title == "" {
				title = s.DisplayName
			}
			if title == "" {
				title = s.Key
			}
			preview := s.LastMessagePreview
			if len(preview) > 60 {
				preview = preview[:60] + "…"
			}
			line := fmt.Sprintf("  %s", title)
			if preview != "" {
				line += fmt.Sprintf(" — %s", preview)
			}
			lines = append(lines, line)
		}
		return gateway.StatusResultMsg{Payload: json.RawMessage(`"` + strings.ReplaceAll(strings.Join(lines, "\n"), `"`, `\"`) + `"`)}
	}
}

func (m *Model) refreshSessionInfo() tea.Cmd {
	client := m.client
	agentID := m.currentAgentID
	return func() tea.Msg {
		result, err := client.ListSessions(gateway.SessionsListParams{
			AgentID: agentID,
		})
		return gateway.SessionsListMsg{Result: result, Err: err}
	}
}

func (m *Model) fetchStatus() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		payload, err := client.GetStatus()
		return gateway.StatusResultMsg{Payload: payload, Err: err}
	}
}

func (m *Model) fetchModels() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		result, err := client.ListModels()
		return gateway.ModelsListMsg{Result: result, Err: err}
	}
}

func (m *Model) resetSession() tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		err := client.ResetSession(sessionKey, "reset")
		return gateway.SessionResetMsg{Err: err}
	}
}

func (m *Model) patchModel(model string) tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		result, err := client.PatchSession(gateway.SessionsPatchParams{
			Key:   sessionKey,
			Model: &model,
		})
		if err == nil {
			return gateway.SessionPatchedMsg{Result: result}
		}
		return gateway.SessionPatchedMsg{Err: err}
	}
}

func (m *Model) patchThinking(level string) tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		result, err := client.PatchSession(gateway.SessionsPatchParams{
			Key:           sessionKey,
			ThinkingLevel: &level,
		})
		if err == nil {
			return gateway.SessionPatchedMsg{Result: result}
		}
		return gateway.SessionPatchedMsg{Err: err}
	}
}

// --- Text extraction from message records ---

func extractText(record gateway.MessageRecord) string {
	// Try content as string
	var str string
	if err := json.Unmarshal(record.Content, &str); err == nil {
		return strings.TrimSpace(str)
	}

	// Try content as array of blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(record.Content, &blocks); err != nil {
		return ""
	}

	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// --- Status formatting ---

func formatStatus(raw json.RawMessage) string {
	var status struct {
		RuntimeVersion  string   `json:"runtimeVersion"`
		ProviderSummary []string `json:"providerSummary"`
		Sessions        *struct {
			Count    int    `json:"count"`
			Defaults *struct {
				Model string `json:"model"`
			} `json:"defaults"`
		} `json:"sessions"`
	}

	if err := json.Unmarshal(raw, &status); err != nil {
		// Fall back to raw display
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return string(raw)
	}

	lines := []string{"Gateway status"}
	if status.RuntimeVersion != "" {
		lines = append(lines, fmt.Sprintf("Version: %s", status.RuntimeVersion))
	}
	for _, line := range status.ProviderSummary {
		lines = append(lines, fmt.Sprintf("  %s", line))
	}
	if status.Sessions != nil {
		lines = append(lines, fmt.Sprintf("Active sessions: %d", status.Sessions.Count))
		if status.Sessions.Defaults != nil && status.Sessions.Defaults.Model != "" {
			lines = append(lines, fmt.Sprintf("Default model: %s", status.Sessions.Defaults.Model))
		}
	}
	return strings.Join(lines, "\n")
}

func helpText() string {
	return `Slash commands:
  /help              Show this help
  /status            Show gateway status
  /agent <id>        Switch agent
  /agents            List agents
  /session <key>     Switch session
  /sessions          List sessions
  /model <p/model>   Set model
  /models            List models
  /think <level>     Set thinking level
  /new               New session
  /reset             Reset session
  /abort             Abort active run
  /exit              Exit

Keyboard:
  Enter              Send message
  Alt+Enter          Newline
  Ctrl+C             Clear/exit
  Ctrl+D             Exit`
}