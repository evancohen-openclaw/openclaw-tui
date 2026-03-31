package gateway

import "encoding/json"

// Frame types for the v3 protocol.

// RequestFrame is sent client → server.
type RequestFrame struct {
	Type   string      `json:"type"`   // always "req"
	ID     string      `json:"id"`     // UUID
	Method string      `json:"method"` // RPC method name
	Params interface{} `json:"params,omitempty"`
}

// ResponseFrame is received server → client.
type ResponseFrame struct {
	Type    string          `json:"type"` // always "res"
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *ErrorShape     `json:"error,omitempty"`
}

// EventFrame is received server → client (push).
type EventFrame struct {
	Type    string          `json:"type"` // always "event"
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Seq     *int            `json:"seq,omitempty"`
}

// ChallengeFrame is the first message from server after WS open.
// It arrives as: {"type":"event","event":"connect.challenge","payload":{"nonce":"…","ts":…}}
type ChallengeFrame struct {
	Type    string           `json:"type"`    // "event"
	Event   string           `json:"event"`   // "connect.challenge"
	Payload ChallengePayload `json:"payload"`
}

type ChallengePayload struct {
	Nonce string `json:"nonce"`
	Ts    int64  `json:"ts"`
}

// ErrorShape is the error object in a response.
type ErrorShape struct {
	Code    string      `json:"code,omitempty"`
	Message string      `json:"message,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

// GenericFrame is used to peek at the "type" field.
type GenericFrame struct {
	Type string `json:"type"`
}

// ConnectParams is sent as params to the "connect" method.
type ConnectParams struct {
	MinProtocol int                `json:"minProtocol"`
	MaxProtocol int                `json:"maxProtocol"`
	Client      ClientInfo         `json:"client"`
	Role        string             `json:"role"`
	Scopes      []string           `json:"scopes"`
	Caps        []string           `json:"caps"`
	Auth        *ConnectAuth       `json:"auth,omitempty"`
	Device      *DeviceConnectInfo `json:"device,omitempty"`
}

type ClientInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	Mode        string `json:"mode"`
	InstanceID  string `json:"instanceId,omitempty"`
}

type ConnectAuth struct {
	Token    string `json:"token,omitempty"`
	Password string `json:"password,omitempty"`
}

// HelloOk is the payload of a successful connect response.
type HelloOk struct {
	Protocol int             `json:"protocol"`
	Server   json.RawMessage `json:"server,omitempty"`
	Snapshot json.RawMessage `json:"snapshot,omitempty"`
	Features json.RawMessage `json:"features,omitempty"`
	Policy   *HelloPolicy    `json:"policy,omitempty"`
	Auth     json.RawMessage `json:"auth,omitempty"`
}

type HelloPolicy struct {
	MaxPayload     int `json:"maxPayload,omitempty"`
	TickIntervalMs int `json:"tickIntervalMs,omitempty"`
}

// --- RPC param/result types ---

type ChatSendParams struct {
	SessionKey     string            `json:"sessionKey"`
	Message        string            `json:"message"`
	Thinking       string            `json:"thinking,omitempty"`
	Deliver        bool              `json:"deliver,omitempty"`
	TimeoutMs      int               `json:"timeoutMs,omitempty"`
	IdempotencyKey string            `json:"idempotencyKey,omitempty"`
	Attachments    []ChatAttachment  `json:"attachments,omitempty"`
}

type ChatAttachment struct {
	Type     string `json:"type"`               // "image"
	MimeType string `json:"mimeType"`           // "image/png", "image/jpeg", etc.
	Content  string `json:"content"`            // base64-encoded data (no data: prefix)
}

type ChatAbortParams struct {
	SessionKey string `json:"sessionKey"`
	RunID      string `json:"runId,omitempty"`
}

type ChatHistoryParams struct {
	SessionKey string `json:"sessionKey"`
	Limit      int    `json:"limit,omitempty"`
}

type ChatHistoryResult struct {
	SessionID     string            `json:"sessionId,omitempty"`
	Messages      []json.RawMessage `json:"messages,omitempty"`
	ThinkingLevel string            `json:"thinkingLevel,omitempty"`
	FastMode      *bool             `json:"fastMode,omitempty"`
	VerboseLevel  string            `json:"verboseLevel,omitempty"`
}

type SessionsListParams struct {
	Limit                int    `json:"limit,omitempty"`
	IncludeGlobal        bool   `json:"includeGlobal,omitempty"`
	IncludeUnknown       bool   `json:"includeUnknown,omitempty"`
	IncludeDerivedTitles bool   `json:"includeDerivedTitles,omitempty"`
	IncludeLastMessage   bool   `json:"includeLastMessage,omitempty"`
	AgentID              string `json:"agentId,omitempty"`
}

type SessionsListResult struct {
	Sessions []SessionEntry  `json:"sessions"`
	Defaults *SessionDefault `json:"defaults,omitempty"`
}

type SessionEntry struct {
	Key                string           `json:"key"`
	DisplayName        string           `json:"displayName,omitempty"`
	DerivedTitle       string           `json:"derivedTitle,omitempty"`
	Label              string           `json:"label,omitempty"`
	Subject            string           `json:"subject,omitempty"`
	SessionID          string           `json:"sessionId,omitempty"`
	UpdatedAt          json.RawMessage  `json:"updatedAt,omitempty"`
	LastMessagePreview string  `json:"lastMessagePreview,omitempty"`
	Model              string  `json:"model,omitempty"`
	ModelProvider      string  `json:"modelProvider,omitempty"`
	ThinkingLevel      string  `json:"thinkingLevel,omitempty"`
	FastMode           *bool   `json:"fastMode,omitempty"`
	VerboseLevel       string  `json:"verboseLevel,omitempty"`
	ReasoningLevel     string  `json:"reasoningLevel,omitempty"`
	ResponseUsage      string  `json:"responseUsage,omitempty"`
	InputTokens        *int    `json:"inputTokens,omitempty"`
	OutputTokens       *int    `json:"outputTokens,omitempty"`
	TotalTokens        *int    `json:"totalTokens,omitempty"`
	ContextTokens      *int    `json:"contextTokens,omitempty"`
}

type SessionDefault struct {
	Model         string `json:"model,omitempty"`
	ModelProvider string `json:"modelProvider,omitempty"`
	ContextTokens *int   `json:"contextTokens,omitempty"`
}

type SessionsPatchParams struct {
	Key            string  `json:"key"`
	Model          *string `json:"model,omitempty"`
	ThinkingLevel  *string `json:"thinkingLevel,omitempty"`
	FastMode       *bool   `json:"fastMode,omitempty"`
	VerboseLevel   *string `json:"verboseLevel,omitempty"`
	ReasoningLevel *string `json:"reasoningLevel,omitempty"`
	ResponseUsage  *string `json:"responseUsage,omitempty"`
	ElevatedLevel  *string `json:"elevatedLevel,omitempty"`
}

type SessionsPatchResult struct {
	Key      string          `json:"key,omitempty"`
	Entry    json.RawMessage `json:"entry,omitempty"`
	Resolved json.RawMessage `json:"resolved,omitempty"`
}

type SessionsResetParams struct {
	Key    string `json:"key"`
	Reason string `json:"reason,omitempty"`
}

type AgentsListResult struct {
	Agents    []AgentEntry `json:"agents"`
	DefaultID string       `json:"defaultId,omitempty"`
	MainKey   string       `json:"mainKey,omitempty"`
	Scope     string       `json:"scope,omitempty"`
}

type AgentEntry struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type ModelsListResult struct {
	Models []ModelEntry `json:"models"`
}

type ModelEntry struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
}

// --- Event payloads ---

type ChatEventPayload struct {
	SessionKey   string          `json:"sessionKey"`
	RunID        string          `json:"runId"`
	State        string          `json:"state"` // delta, final, aborted, error
	Message      json.RawMessage `json:"message,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
}

type AgentEventPayload struct {
	RunID  string          `json:"runId"`
	Stream string          `json:"stream"` // tool, lifecycle
	Data   json.RawMessage `json:"data,omitempty"`
}

type ToolEventData struct {
	Phase         string          `json:"phase"` // start, update, result
	ToolCallID    string          `json:"toolCallId"`
	Name          string          `json:"name"`
	Args          json.RawMessage `json:"args,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	PartialResult json.RawMessage `json:"partialResult,omitempty"`
	IsError       bool            `json:"isError,omitempty"`
}

// MessageRecord is a chat message from history or streaming.
type MessageRecord struct {
	Role         string          `json:"role"`
	Content      json.RawMessage `json:"content"`
	Command      bool            `json:"command,omitempty"`
	StopReason   string          `json:"stopReason,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	ToolCallID   string          `json:"toolCallId,omitempty"`
	ToolName     string          `json:"toolName,omitempty"`
	IsError      bool            `json:"isError,omitempty"`
}
