package gateway

import (
	"encoding/json"
	"fmt"
)

// ChatClient wraps Client with typed RPC methods matching GatewayChatClient.
type ChatClient struct {
	*Client
}

// NewChatClient creates a ChatClient.
func NewChatClient(wsURL, token, password, version string) *ChatClient {
	return &ChatClient{
		Client: NewClient(wsURL, token, password, version),
	}
}

// SendChat sends a message to a session.
func (c *ChatClient) SendChat(sessionKey, message, runID string) error {
	params := ChatSendParams{
		SessionKey:     sessionKey,
		Message:        message,
		IdempotencyKey: runID,
	}
	_, err := c.Request("chat.send", params)
	return err
}

// AbortChat aborts an active chat run.
func (c *ChatClient) AbortChat(sessionKey, runID string) error {
	params := ChatAbortParams{
		SessionKey: sessionKey,
		RunID:      runID,
	}
	_, err := c.Request("chat.abort", params)
	return err
}

// LoadHistory fetches conversation history.
func (c *ChatClient) LoadHistory(sessionKey string, limit int) (*ChatHistoryResult, error) {
	params := ChatHistoryParams{
		SessionKey: sessionKey,
		Limit:      limit,
	}
	raw, err := c.Request("chat.history", params)
	if err != nil {
		return nil, err
	}
	var result ChatHistoryResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse history: %w", err)
	}
	return &result, nil
}

// ListSessions lists sessions.
func (c *ChatClient) ListSessions(params SessionsListParams) (*SessionsListResult, error) {
	raw, err := c.Request("sessions.list", params)
	if err != nil {
		return nil, err
	}
	var result SessionsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse sessions: %w", err)
	}
	return &result, nil
}

// PatchSession updates session settings.
func (c *ChatClient) PatchSession(params SessionsPatchParams) (*SessionsPatchResult, error) {
	raw, err := c.Request("sessions.patch", params)
	if err != nil {
		return nil, err
	}
	var result SessionsPatchResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse patch: %w", err)
	}
	return &result, nil
}

// ResetSession resets a session.
func (c *ChatClient) ResetSession(key, reason string) error {
	params := SessionsResetParams{Key: key, Reason: reason}
	_, err := c.Request("sessions.reset", params)
	return err
}

// ListAgents lists available agents.
func (c *ChatClient) ListAgents() (*AgentsListResult, error) {
	raw, err := c.Request("agents.list", struct{}{})
	if err != nil {
		return nil, err
	}
	var result AgentsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse agents: %w", err)
	}
	return &result, nil
}

// ListModels lists available models.
func (c *ChatClient) ListModels() (*ModelsListResult, error) {
	raw, err := c.Request("models.list", struct{}{})
	if err != nil {
		return nil, err
	}
	var result ModelsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse models: %w", err)
	}
	return &result, nil
}

// GetStatus fetches gateway status.
func (c *ChatClient) GetStatus() (json.RawMessage, error) {
	return c.Request("status", struct{}{})
}
