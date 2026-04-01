package model

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"

	"github.com/evancohen/openclaw-tui/internal/clipboard"
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
	model          string
	modelProvider  string
	thinkingLevel  string
	fastMode       string
	verboseLevel   string
	reasoningLevel string
	responseUsage  string
	elevatedLevel  string
	deliver        bool
	totalTokens    *int
	contextTokens  *int

	// Chat state
	messages     []chatMessage
	activeRunID  string
	assembler    *stream.Assembler
	localRunIDs  map[string]bool
	runStartedAt *time.Time

	// Picker (list component) state
	pickerActive bool
	pickerType   string // "agents", "models", "sessions"
	pickerList   list.Model

	// Autocomplete state
	autocompleteActive bool
	autocompleteSuggs  []string
	autocompleteIndex  int

	// Event channel
	eventCh chan tea.Msg

	// UI toggles
	mouseEnabled  bool
	toolsExpanded bool

	// Pending attachments for next message
	pendingFiles []pendingAttachment

	// Ctrl+C state
	lastCtrlC time.Time
}

// pickerItem implements list.DefaultItem for the picker.
type pickerItem struct {
	id          string
	title_      string
	description string
}

func (i pickerItem) Title() string       { return i.title_ }
func (i pickerItem) Description() string { return i.description }
func (i pickerItem) FilterValue() string { return i.title_ + " " + i.description }

type chatMessage struct {
	role       string // "user", "assistant", "system", "tool-pending", "tool-success", "tool-error", "assistant-stream", "thinking"
	content    string
	toolCallID string // for tool messages, used to find/update them
}

// tickMsg drives periodic updates (elapsed timer, etc.)
type tickMsg time.Time

// pickerReadyMsg signals that picker data has been fetched.
type pickerReadyMsg struct {
	pickerType string
	items      []pickerItem
	err        error
}

func doTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// New creates the root model.
func New(cfg config.Config) Model {
	th := theme.New(cfg.Theme)

	ta := textarea.New()
	ta.Placeholder = "Type a message... (/ for commands)"
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA"))

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
	m.client = gateway.NewChatClient(cfg.URL, cfg.Token, cfg.Password, cfg.Version, config.DefaultConfigDir(), cfg.TLSInsecure)

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

	m.client.OnReconnecting = func(attempt int) {
		eventCh <- gateway.ReconnectingMsg{Attempt: attempt}
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

	case tea.MouseMsg:
		// Forward mouse events (scroll wheel) to the viewport
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

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
		m.activityStatus = "disconnected"
		cmds = append(cmds, waitForEvent(m.eventCh))
		return m, tea.Batch(cmds...)

	case gateway.ReconnectingMsg:
		m.connStatus = fmt.Sprintf("reconnecting (attempt %d)", msg.Attempt)
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

	case clipboardImageMsg:
		if msg.err != nil {
			// No image on clipboard — that's fine, normal paste will handle text
			return m, nil
		}
		m.pendingFiles = append(m.pendingFiles, *msg.attachment)
		m.addSystem(fmt.Sprintf("📎 %s attached — type a message and press Enter to send", msg.attachment.name))
		return m, nil

	case pickerReadyMsg:
		if msg.err != nil {
			m.addSystem(fmt.Sprintf("%s list failed: %v", msg.pickerType, msg.err))
		} else if len(msg.items) > 0 {
			items := make([]list.Item, len(msg.items))
			for i, item := range msg.items {
				items[i] = item
			}

			w := m.width * 2 / 3
			if w < 40 {
				w = 40
			}
			h := m.height * 2 / 3
			if h < 10 {
				h = 10
			}

			delegate := list.NewDefaultDelegate()
			l := list.New(items, delegate, w, h)
			l.Title = "Select " + msg.pickerType
			l.SetShowStatusBar(true)
			l.SetFilteringEnabled(true)
			l.SetShowHelp(true)

			m.pickerList = l
			m.pickerActive = true
			m.pickerType = msg.pickerType
		} else {
			m.addSystem(fmt.Sprintf("no %s found", msg.pickerType))
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
	// Picker takes priority over all other input
	if m.pickerActive {
		return m.handlePickerKey(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		if m.autocompleteActive {
			m.autocompleteActive = false
			return m, nil
		}
		return m.handleCtrlC()
	case "ctrl+d":
		m.client.Stop()
		return m, tea.Quit
	case "tab":
		if m.autocompleteActive && len(m.autocompleteSuggs) > 0 {
			// Complete the selected suggestion
			m.input.Reset()
			m.input.SetValue("/" + m.autocompleteSuggs[m.autocompleteIndex] + " ")
			m.autocompleteActive = false
			return m, nil
		}
	case "shift+enter", "alt+enter", "ctrl+j":
		// Insert a newline
		m.input.InsertString("\n")
		m.resizeInput()
		return m, nil
	case "ctrl+t":
		// Toggle thinking visibility (matches official TUI)
		m.assembler.ShowThinking = !m.assembler.ShowThinking
		if m.assembler.ShowThinking {
			m.addSystem("thinking: visible")
		} else {
			m.addSystem("thinking: hidden")
		}
		return m, m.loadHistory()
	case "enter":
		if m.autocompleteActive && len(m.autocompleteSuggs) > 0 {
			m.input.Reset()
			m.input.SetValue("/" + m.autocompleteSuggs[m.autocompleteIndex])
			m.autocompleteActive = false
			text := strings.TrimSpace(m.input.Value())
			if text != "" {
				m.input.Reset()
				m.resizeInput()
				return m.handleSubmit(text)
			}
			return m, nil
		}
		if m.input.Focused() {
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			m.resizeInput()
			m.autocompleteActive = false
			return m.handleSubmit(text)
		}
	case "up":
		if m.autocompleteActive && len(m.autocompleteSuggs) > 0 {
			m.autocompleteIndex--
			if m.autocompleteIndex < 0 {
				m.autocompleteIndex = len(m.autocompleteSuggs) - 1
			}
			return m, nil
		}
	case "down":
		if m.autocompleteActive && len(m.autocompleteSuggs) > 0 {
			m.autocompleteIndex++
			if m.autocompleteIndex >= len(m.autocompleteSuggs) {
				m.autocompleteIndex = 0
			}
			return m, nil
		}
	case "alt+v":
		// Paste image from clipboard
		return m, m.pasteClipboardImage()
	case "ctrl+l":
		// Model picker (matches official TUI)
		return m, m.fetchModelsOverlay()
	case "ctrl+g":
		// Agent picker (matches official TUI)
		return m, m.fetchAgentsOverlay()
	case "ctrl+p":
		// Session picker (matches official TUI)
		return m, m.fetchSessionsOverlay()
	case "ctrl+o":
		// Toggle tool output expansion (TODO: implement expanded/collapsed tool cards)
		m.toolsExpanded = !m.toolsExpanded
		if m.toolsExpanded {
			m.addSystem("tool output: expanded")
		} else {
			m.addSystem("tool output: collapsed")
		}
		return m, nil
	case "escape":
		if m.autocompleteActive {
			m.autocompleteActive = false
			return m, nil
		}
		// Abort active run (matches official TUI)
		if m.activeRunID != "" {
			return m, m.abortChat()
		}
	case "pgup":
		m.viewport.HalfPageUp()
		return m, nil
	case "pgdown":
		m.viewport.HalfPageDown()
		return m, nil
	case "home":
		m.viewport.GotoTop()
		return m, nil
	case "end":
		m.viewport.GotoBottom()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	// Resize input area based on content
	m.resizeInput()

	// Update autocomplete based on current input
	m.updateAutocomplete()

	return m, cmd
}

var slashCommands = []string{
	"help", "exit", "quit", "new", "reset", "abort", "status",
	"model", "models", "agent", "agents", "session", "sessions",
	"think", "fast", "verbose", "reasoning", "usage", "elevated", "elev",
	"deliver", "settings", "clear", "mouse", "config", "attach", "file",
}

func (m *Model) updateAutocomplete() {
	val := m.input.Value()
	if !strings.HasPrefix(val, "/") || strings.Contains(val, " ") {
		m.autocompleteActive = false
		return
	}

	prefix := strings.ToLower(strings.TrimPrefix(val, "/"))
	if prefix == "" {
		// Show all commands
		m.autocompleteSuggs = slashCommands
		m.autocompleteActive = true
		m.autocompleteIndex = 0
		return
	}

	var matches []string
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd, prefix) {
			matches = append(matches, cmd)
		}
	}

	if len(matches) == 0 {
		m.autocompleteActive = false
		return
	}

	m.autocompleteSuggs = matches
	m.autocompleteActive = true
	if m.autocompleteIndex >= len(matches) {
		m.autocompleteIndex = 0
	}
}

func (m *Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "escape":
		// If filtering, let list handle it; otherwise close picker
		if m.pickerList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.pickerList, cmd = m.pickerList.Update(msg)
			return m, cmd
		}
		m.pickerActive = false
		return m, nil
	case "ctrl+c":
		m.pickerActive = false
		return m, nil
	case "enter":
		if m.pickerList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.pickerList, cmd = m.pickerList.Update(msg)
			return m, cmd
		}
		item, ok := m.pickerList.SelectedItem().(pickerItem)
		if !ok {
			m.pickerActive = false
			return m, nil
		}
		m.pickerActive = false
		return m.handlePickerSelect(item)
	}

	// Let the list handle everything else (filtering, nav)
	var cmd tea.Cmd
	m.pickerList, cmd = m.pickerList.Update(msg)
	return m, cmd
}

func (m *Model) handlePickerSelect(item pickerItem) (tea.Model, tea.Cmd) {
	switch m.pickerType {
	case "agents":
		m.currentAgentID = item.id
		m.currentSessionKey = m.resolveSessionKey("")
		m.messages = nil
		m.assembler.Reset()
		m.addSystem(fmt.Sprintf("switched to agent: %s", item.id))
		return m, m.loadHistory()
	case "models":
		m.addSystem(fmt.Sprintf("switching model to %s", item.id))
		return m, m.patchModel(item.id)
	case "sessions":
		m.currentSessionKey = m.resolveSessionKey(item.id)
		m.messages = nil
		m.assembler.Reset()
		m.activeRunID = ""
		m.addSystem(fmt.Sprintf("switched to session: %s", item.title_))
		return m, m.loadHistory()
	}
	return m, nil
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

	// Extract file attachments from the message text
	message, attachments := m.extractAttachments(text)

	// Add any pending attachments from /attach
	attachments = append(attachments, m.pendingFiles...)
	m.pendingFiles = nil

	if message == "" && len(attachments) > 0 {
		message = fmt.Sprintf("[%d file(s) attached]", len(attachments))
	}

	displayText := message
	if len(attachments) > 0 {
		for _, a := range attachments {
			displayText += fmt.Sprintf("\n📎 %s", a.name)
		}
	}
	m.addUser(displayText)

	runID := uuid.New().String()
	m.activeRunID = runID
	m.localRunIDs[runID] = true
	m.activityStatus = actSending
	now := time.Now()
	m.runStartedAt = &now

	var chatAttachments []gateway.ChatAttachment
	for _, a := range attachments {
		chatAttachments = append(chatAttachments, a.attachment)
	}

	return m, m.sendChat(message, runID, chatAttachments)
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
			return m, m.fetchModelsOverlay()
		}
		return m, m.patchModel(args)

	case "models":
		return m, m.fetchModelsOverlay()

	case "agent":
		if args == "" {
			return m, m.fetchAgentsOverlay()
		}
		m.currentAgentID = args
		m.currentSessionKey = m.resolveSessionKey("")
		m.messages = nil
		m.assembler.Reset()
		return m, m.loadHistory()

	case "agents":
		return m, m.fetchAgentsOverlay()

	case "session":
		if args == "" {
			return m, m.fetchSessionsOverlay()
		}
		m.currentSessionKey = m.resolveSessionKey(args)
		m.messages = nil
		m.assembler.Reset()
		m.activeRunID = ""
		return m, m.loadHistory()

	case "sessions":
		return m, m.fetchSessionsOverlay()

	case "think":
		if args == "" {
			m.addSystem(fmt.Sprintf("thinking: %s", m.thinkingLevel))
			return m, nil
		}
		return m, m.patchThinking(args)

	case "fast":
		if args == "" || args == "status" {
			m.addSystem(fmt.Sprintf("fast mode: %s", orDefault(m.fastMode, "off")))
			return m, nil
		}
		return m, m.patchSessionField("fastMode", args == "on")

	case "verbose":
		if args == "" {
			m.addSystem(fmt.Sprintf("verbose: %s", orDefault(m.verboseLevel, "off")))
			return m, nil
		}
		return m, m.patchVerbose(args)

	case "reasoning":
		if args == "" {
			m.addSystem(fmt.Sprintf("reasoning: %s", orDefault(m.reasoningLevel, "off")))
			return m, nil
		}
		return m, m.patchReasoning(args)

	case "usage":
		if args == "" {
			m.addSystem(fmt.Sprintf("usage display: %s", orDefault(m.responseUsage, "off")))
			return m, nil
		}
		return m, m.patchUsage(args)

	case "elevated", "elev":
		if args == "" {
			m.addSystem(fmt.Sprintf("elevated: %s", orDefault(m.elevatedLevel, "off")))
			return m, nil
		}
		return m, m.patchElevated(args)

	case "deliver":
		if args == "" {
			m.addSystem(fmt.Sprintf("deliver: %v", m.deliver))
			return m, nil
		}
		m.deliver = args == "on"
		m.addSystem(fmt.Sprintf("deliver: %v", m.deliver))
		return m, nil

	case "settings":
		lines := []string{
			"Settings",
			fmt.Sprintf("  model:     %s/%s", m.modelProvider, m.model),
			fmt.Sprintf("  thinking:  %s", orDefault(m.thinkingLevel, "off")),
			fmt.Sprintf("  fast:      %s", orDefault(m.fastMode, "off")),
			fmt.Sprintf("  verbose:   %s", orDefault(m.verboseLevel, "off")),
			fmt.Sprintf("  reasoning: %s", orDefault(m.reasoningLevel, "off")),
			fmt.Sprintf("  elevated:  %s", orDefault(m.elevatedLevel, "off")),
			fmt.Sprintf("  deliver:   %v", m.deliver),
		}
		m.addSystem(strings.Join(lines, "\n"))
		return m, nil

	case "clear":
		m.messages = nil
		m.updateViewport()
		return m, nil

	case "attach", "file":
		if args == "" {
			if len(m.pendingFiles) > 0 {
				var names []string
				for _, f := range m.pendingFiles {
					names = append(names, f.name)
				}
				m.addSystem(fmt.Sprintf("pending attachments: %s\nUse /attach <path> to add, or type a message to send with attachments", strings.Join(names, ", ")))
			} else {
				m.addSystem("usage: /attach <file-path>\nAttach an image to your next message. Send a message after to include it.")
			}
			return m, nil
		}
		if args == "clear" {
			m.pendingFiles = nil
			m.addSystem("cleared pending attachments")
			return m, nil
		}
		expanded := expandPath(strings.TrimSpace(args))
		att, err := loadFileAttachment(expanded)
		if err != nil {
			m.addSystem(fmt.Sprintf("failed to attach: %v", err))
			return m, nil
		}
		m.pendingFiles = append(m.pendingFiles, *att)
		m.addSystem(fmt.Sprintf("📎 attached %s (%s) — type a message to send", att.name, att.attachment.MimeType))
		return m, nil

	case "mouse":
		m.mouseEnabled = !m.mouseEnabled
		if m.mouseEnabled {
			m.addSystem("mouse scrolling enabled (copy/paste disabled)")
		} else {
			m.addSystem("mouse scrolling disabled (copy/paste enabled)")
		}
		return m, nil

	case "config":
		lines := []string{
			"Config",
			fmt.Sprintf("  file:     %s", config.DefaultConfigPath()),
			fmt.Sprintf("  url:      %s", m.cfg.URL),
			fmt.Sprintf("  theme:    %s", orDefault(m.cfg.Theme, "dark")),
			fmt.Sprintf("  session:  %s", orDefault(m.cfg.Session, "(auto)")),
		}
		if m.cfg.Token != "" {
			lines = append(lines, fmt.Sprintf("  token:    %s…", m.cfg.Token[:min(8, len(m.cfg.Token))]))
		}
		m.addSystem(strings.Join(lines, "\n"))
		return m, nil

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
		return m, m.sendChat(raw, runID, nil)
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
	switch evt.Stream {
	case "lifecycle":
		var data struct {
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal(evt.Data, &data); err == nil {
			switch data.Phase {
			case "start":
				m.activityStatus = actRunning
			case "end":
				if m.activityStatus == actRunning {
					m.activityStatus = actIdle
				}
			case "error":
				m.activityStatus = actError
			}
		}

	case "tool":
		var tool gateway.ToolEventData
		if err := json.Unmarshal(evt.Data, &tool); err != nil {
			return
		}
		m.handleToolEvent(tool)
		m.updateViewport()
	}
}

func (m *Model) handleToolEvent(tool gateway.ToolEventData) {
	switch tool.Phase {
	case "start":
		args := truncate(string(tool.Args), 80)
		content := fmt.Sprintf("⚙ %s(%s)", tool.Name, args)
		m.messages = append(m.messages, chatMessage{
			role:       "tool-pending",
			content:    content,
			toolCallID: tool.ToolCallID,
		})

	case "update":
		partial := truncate(string(tool.PartialResult), 200)
		content := fmt.Sprintf("⚙ %s: %s", tool.Name, partial)
		m.updateToolMessage(tool.ToolCallID, "tool-pending", content)

	case "result":
		if tool.IsError {
			output := truncate(string(tool.Result), 400)
			content := fmt.Sprintf("✗ %s: %s", tool.Name, output)
			m.updateToolMessage(tool.ToolCallID, "tool-error", content)
		} else {
			output := truncateLines(string(tool.Result), 10)
			content := fmt.Sprintf("✓ %s: %s", tool.Name, output)
			m.updateToolMessage(tool.ToolCallID, "tool-success", content)
		}
	}
}

func (m *Model) updateToolMessage(toolCallID, role, content string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].toolCallID == toolCallID {
			m.messages[i].role = role
			m.messages[i].content = content
			return
		}
	}
	// Not found, append
	m.messages = append(m.messages, chatMessage{role: role, content: content, toolCallID: toolCallID})
}

func truncate(s string, maxLen int) string {
	// Clean up JSON quoting for display
	s = strings.TrimSpace(s)
	if len(s) > 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var unquoted string
		if err := json.Unmarshal([]byte(s), &unquoted); err == nil {
			s = unquoted
		}
	}
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}

func truncateLines(s string, maxLines int) string {
	s = strings.TrimSpace(s)
	if len(s) > 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var unquoted string
		if err := json.Unmarshal([]byte(s), &unquoted); err == nil {
			s = unquoted
		}
	}
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], fmt.Sprintf("… (%d more lines)", len(lines)-maxLines))
	}
	return strings.Join(lines, "\n")
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
	// If thinking is enabled and there's thinking text, insert a thinking block
	if m.assembler.ShowThinking {
		thinkingText := m.assembler.GetThinking(runID)
		if strings.TrimSpace(thinkingText) != "" {
			// Strip the thinking prefix from the display text since we show it separately
			// The compose() function already put it in text, so we need the raw content only
			// Actually, compose() already included it. We want to show thinking in its own block.
			// So we insert a thinking message before the assistant message.
			// Find where to insert (before the streaming message or at end)
			insertIdx := len(m.messages)
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].role == "assistant-stream" {
					insertIdx = i
					break
				}
			}
			thinkMsg := chatMessage{role: "thinking", content: thinkingText}
			// Insert thinking before the streaming message
			m.messages = append(m.messages[:insertIdx], append([]chatMessage{thinkMsg}, m.messages[insertIdx:]...)...)
		}
	}

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
		m.assembler.ShowThinking = m.thinkingLevel != "" && m.thinkingLevel != "off"
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
				m.assembler.ShowThinking = m.thinkingLevel != "" && m.thinkingLevel != "off"
			}
			if s.FastMode != nil {
				if *s.FastMode {
					m.fastMode = "on"
				} else {
					m.fastMode = "off"
				}
			}
			if s.VerboseLevel != "" {
				m.verboseLevel = s.VerboseLevel
			}
			if s.ReasoningLevel != "" {
				m.reasoningLevel = s.ReasoningLevel
			}
			if s.ResponseUsage != "" {
				m.responseUsage = s.ResponseUsage
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

func (m *Model) sendChat(text, runID string, attachments []gateway.ChatAttachment) tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		err := client.SendChat(sessionKey, text, runID, attachments)
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

func (m *Model) fetchAgentsOverlay() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		result, err := client.ListAgents()
		if err != nil {
			return pickerReadyMsg{pickerType: "agents", err: err}
		}
		var items []pickerItem
		for _, a := range result.Agents {
			name := a.Name
			if name == "" {
				name = a.ID
			}
			desc := a.ID
			if a.ID == result.DefaultID {
				desc += " (default)"
			}
			items = append(items, pickerItem{id: a.ID, title_: name, description: desc})
		}
		return pickerReadyMsg{pickerType: "agents", items: items}
	}
}

func (m *Model) fetchSessionsOverlay() tea.Cmd {
	client := m.client
	agentID := m.currentAgentID
	return func() tea.Msg {
		result, err := client.ListSessions(gateway.SessionsListParams{
			IncludeDerivedTitles: true,
			IncludeLastMessage:   true,
			AgentID:              agentID,
		})
		if err != nil {
			return pickerReadyMsg{pickerType: "sessions", err: err}
		}
		var items []pickerItem
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
			sessionName := s.Key
			if strings.HasPrefix(s.Key, "agent:") {
				parts := strings.SplitN(s.Key, ":", 3)
				if len(parts) == 3 {
					sessionName = parts[2]
				}
			}
			items = append(items, pickerItem{id: sessionName, title_: title, description: preview})
		}
		return pickerReadyMsg{pickerType: "sessions", items: items}
	}
}

func (m *Model) fetchModelsOverlay() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		result, err := client.ListModels()
		if err != nil {
			return pickerReadyMsg{pickerType: "models", err: err}
		}
		var items []pickerItem
		for _, model := range result.Models {
			id := model.Provider + "/" + model.ID
			name := model.Name
			if name == "" || name == model.ID {
				name = id
			}
			items = append(items, pickerItem{id: id, title_: name, description: id})
		}
		return pickerReadyMsg{pickerType: "models", items: items}
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

func (m *Model) patchVerbose(level string) tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		result, err := client.PatchSession(gateway.SessionsPatchParams{
			Key:          sessionKey,
			VerboseLevel: &level,
		})
		if err == nil {
			return gateway.SessionPatchedMsg{Result: result}
		}
		return gateway.SessionPatchedMsg{Err: err}
	}
}

func (m *Model) patchReasoning(level string) tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		result, err := client.PatchSession(gateway.SessionsPatchParams{
			Key:            sessionKey,
			ReasoningLevel: &level,
		})
		if err == nil {
			return gateway.SessionPatchedMsg{Result: result}
		}
		return gateway.SessionPatchedMsg{Err: err}
	}
}

func (m *Model) patchUsage(level string) tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		result, err := client.PatchSession(gateway.SessionsPatchParams{
			Key:           sessionKey,
			ResponseUsage: &level,
		})
		if err == nil {
			return gateway.SessionPatchedMsg{Result: result}
		}
		return gateway.SessionPatchedMsg{Err: err}
	}
}

func (m *Model) patchElevated(level string) tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		result, err := client.PatchSession(gateway.SessionsPatchParams{
			Key:           sessionKey,
			ElevatedLevel: &level,
		})
		if err == nil {
			return gateway.SessionPatchedMsg{Result: result}
		}
		return gateway.SessionPatchedMsg{Err: err}
	}
}

func (m *Model) patchSessionField(field string, value bool) tea.Cmd {
	client := m.client
	sessionKey := m.currentSessionKey
	return func() tea.Msg {
		params := gateway.SessionsPatchParams{Key: sessionKey}
		switch field {
		case "fastMode":
			params.FastMode = &value
		}
		result, err := client.PatchSession(params)
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

// clipboardImageMsg is sent when a clipboard image check completes.
type clipboardImageMsg struct {
	attachment *pendingAttachment
	err        error
}

type pendingAttachment struct {
	name       string
	attachment gateway.ChatAttachment
}

// extractAttachments scans the message for file paths and loads them as attachments.
// Returns the cleaned message text and any attachments found.
func (m *Model) extractAttachments(text string) (string, []pendingAttachment) {
	var attachments []pendingAttachment
	var cleanedLines []string

	// First check if the entire message (minus whitespace) is a raw base64 blob or data URL
	trimmedAll := strings.TrimSpace(text)
	if isDataURL(trimmedAll) {
		if mimeType, data, ok := parseDataURL(trimmedAll); ok {
			attachments = append(attachments, pendingAttachment{
				name: "pasted image",
				attachment: gateway.ChatAttachment{
					Type:     "image",
					MimeType: mimeType,
					Content:  data,
				},
			})
			return "", attachments
		}
	}
	if isRawBase64Image(trimmedAll) && !strings.Contains(trimmedAll, " ") && len(trimmedAll) > 100 {
		attachments = append(attachments, pendingAttachment{
			name: "pasted image",
			attachment: gateway.ChatAttachment{
				Type:     "image",
				MimeType: mimeFromBase64Prefix(trimmedAll),
				Content:  trimmedAll,
			},
		})
		return "", attachments
	}

	// Otherwise scan line by line for file paths
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if isFilePath(trimmed) {
			expanded := expandPath(trimmed)
			att, err := loadFileAttachment(expanded)
			if err != nil {
				m.addSystem(fmt.Sprintf("failed to attach %s: %v", trimmed, err))
				cleanedLines = append(cleanedLines, line)
				continue
			}
			attachments = append(attachments, *att)
		} else if isDataURL(trimmed) {
			if mimeType, data, ok := parseDataURL(trimmed); ok {
				attachments = append(attachments, pendingAttachment{
					name: "pasted image",
					attachment: gateway.ChatAttachment{
						Type:     "image",
						MimeType: mimeType,
						Content:  data,
					},
				})
			} else {
				cleanedLines = append(cleanedLines, line)
			}
		} else {
			cleanedLines = append(cleanedLines, line)
		}
	}

	return strings.TrimSpace(strings.Join(cleanedLines, "\n")), attachments
}

func isFilePath(s string) bool {
	if s == "" {
		return false
	}
	if !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "~/") && !strings.HasPrefix(s, "./") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(s))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	}
	return false
}

func isDataURL(s string) bool {
	return strings.HasPrefix(s, "data:image/")
}

func isRawBase64Image(s string) bool {
	// PNG starts with iVBOR, JPEG with /9j/, GIF with R0lGOD, WebP with UklGR
	return strings.HasPrefix(s, "iVBOR") || // PNG
		strings.HasPrefix(s, "/9j/") || // JPEG
		strings.HasPrefix(s, "R0lGOD") || // GIF
		strings.HasPrefix(s, "UklGR") // WebP
}

func mimeFromBase64Prefix(s string) string {
	switch {
	case strings.HasPrefix(s, "iVBOR"):
		return "image/png"
	case strings.HasPrefix(s, "/9j/"):
		return "image/jpeg"
	case strings.HasPrefix(s, "R0lGOD"):
		return "image/gif"
	case strings.HasPrefix(s, "UklGR"):
		return "image/webp"
	default:
		return "image/png"
	}
}

func parseDataURL(s string) (mimeType, data string, ok bool) {
	// data:image/png;base64,iVBOR...
	if !strings.HasPrefix(s, "data:") {
		return "", "", false
	}
	parts := strings.SplitN(s[5:], ",", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	meta := parts[0] // "image/png;base64"
	mimeType = strings.TrimSuffix(meta, ";base64")
	return mimeType, parts[1], true
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func loadFileAttachment(path string) (*pendingAttachment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 20MB limit
	if len(data) > 20*1024*1024 {
		return nil, fmt.Errorf("file too large (%d MB, max 20MB)", len(data)/1024/1024)
	}

	ext := strings.ToLower(filepath.Ext(path))
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		switch ext {
		case ".png":
			mimeType = "image/png"
		case ".jpg", ".jpeg":
			mimeType = "image/jpeg"
		case ".gif":
			mimeType = "image/gif"
		case ".webp":
			mimeType = "image/webp"
		case ".bmp":
			mimeType = "image/bmp"
		case ".svg":
			mimeType = "image/svg+xml"
		default:
			mimeType = "application/octet-stream"
		}
	}

	encoded := base64.StdEncoding.EncodeToString(data)

	return &pendingAttachment{
		name: filepath.Base(path),
		attachment: gateway.ChatAttachment{
			Type:     "image",
			MimeType: mimeType,
			Content:  encoded,
		},
	}, nil
}

func (m *Model) pasteClipboardImage() tea.Cmd {
	return func() tea.Msg {
		path, mimeType, err := clipboard.GetImage()
		if err != nil {
			return clipboardImageMsg{err: err}
		}
		defer os.Remove(path)

		data, err := os.ReadFile(path)
		if err != nil {
			return clipboardImageMsg{err: fmt.Errorf("read clipboard image: %w", err)}
		}

		if len(data) > 20*1024*1024 {
			return clipboardImageMsg{err: fmt.Errorf("clipboard image too large (%d MB)", len(data)/1024/1024)}
		}

		encoded := base64.StdEncoding.EncodeToString(data)
		att := &pendingAttachment{
			name: "clipboard image",
			attachment: gateway.ChatAttachment{
				Type:     "image",
				MimeType: mimeType,
				Content:  encoded,
			},
		}
		return clipboardImageMsg{attachment: att}
	}
}

// resizeInput adjusts the textarea height based on content (min 3, max 10 lines).
func (m *Model) resizeInput() {
	val := m.input.Value()
	lines := strings.Count(val, "\n") + 1
	h := lines + 1 // extra line for padding
	if h < 3 {
		h = 3
	}
	if h > 10 {
		h = 10
	}
	m.input.SetHeight(h)
	m.updateLayout()
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func helpText() string {
	return `Slash commands:
  /help              Show this help
  /status            Gateway status
  /agent [id]        Agent picker / switch
  /session [key]     Session picker / switch
  /model [p/model]   Model picker / set
  /think <level>     off|minimal|low|medium|high
  /fast <on|off>     Toggle fast mode
  /verbose <on|off>  Toggle verbose
  /reasoning <on|off|stream>
  /usage <off|tokens|full>
  /elevated <on|off|ask|full>
  /deliver <on|off>  Toggle delivery
  /settings          Show all settings
  /new               New session
  /reset             Reset session
  /clear             Clear display
  /abort             Abort run
  /mouse             Toggle mouse scroll
  /config            Show config
  /exit              Exit

Keyboard:
  Enter              Send message
  Tab                Accept autocomplete
  PgUp/PgDn          Scroll chat
  Home/End           Jump top/bottom
  Escape             Dismiss popup
  Ctrl+C             Clear / double-tap exit
  Ctrl+D             Exit`
}