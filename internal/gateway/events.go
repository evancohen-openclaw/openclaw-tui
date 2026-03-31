package gateway

import (
	"encoding/json"

	tea "charm.land/bubbletea/v2"
)

// Tea messages produced by gateway events.

type ConnectedMsg struct {
	Hello *HelloOk
}

type DisconnectedMsg struct {
	Reason string
}

type ChatEventMsg struct {
	ChatEventPayload
}

type AgentEventMsg struct {
	AgentEventPayload
}

type GatewayErrorMsg struct {
	Err error
}

func (e GatewayErrorMsg) Error() string { return e.Err.Error() }

// RPC result messages

type SessionsListMsg struct {
	Result *SessionsListResult
	Err    error
}

type AgentsListMsg struct {
	Result *AgentsListResult
	Err    error
}

type ModelsListMsg struct {
	Result *ModelsListResult
	Err    error
}

type HistoryLoadedMsg struct {
	Result *ChatHistoryResult
	Err    error
}

type ChatSentMsg struct {
	RunID string
	Err   error
}

type StatusResultMsg struct {
	Payload json.RawMessage
	Err     error
}

type SessionPatchedMsg struct {
	Result *SessionsPatchResult
	Err    error
}

type SessionResetMsg struct {
	Err error
}

// ListenForEvents returns a tea.Cmd that listens for gateway events
// and dispatches them as tea.Msgs. This should be called once after
// the client is set up.
func ListenForEvents(client *ChatClient, eventCh <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-eventCh
	}
}
