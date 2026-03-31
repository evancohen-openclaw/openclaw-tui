package stream

import (
	"encoding/json"
	"strings"
)

// Assembler tracks per-run streaming state to compose display text.
type Assembler struct {
	runs         map[string]*runState
	ShowThinking bool
}

type runState struct {
	ThinkingText string
	ContentText  string
	DisplayText  string
}

// New creates a new stream assembler.
func New() *Assembler {
	return &Assembler{
		runs: make(map[string]*runState),
	}
}

// IngestDelta processes a streaming delta and returns the updated display text,
// or empty string if nothing changed.
func (a *Assembler) IngestDelta(runID string, message json.RawMessage) string {
	state := a.getOrCreate(runID)

	thinking, content := extractFromMessage(message)
	if thinking != "" {
		state.ThinkingText = thinking
	}
	if content != "" {
		state.ContentText = content
	}

	prev := state.DisplayText
	state.DisplayText = compose(state.ThinkingText, state.ContentText, a.ShowThinking)

	if state.DisplayText == prev || state.DisplayText == "" {
		return ""
	}
	return state.DisplayText
}

// Finalize processes the final message for a run and returns the display text.
func (a *Assembler) Finalize(runID string, message json.RawMessage, errorMessage string) string {
	state := a.getOrCreate(runID)

	thinking, content := extractFromMessage(message)
	if thinking != "" {
		state.ThinkingText = thinking
	}
	if content != "" {
		state.ContentText = content
	}

	state.DisplayText = compose(state.ThinkingText, state.ContentText, a.ShowThinking)

	finalText := state.DisplayText
	if strings.TrimSpace(finalText) == "" {
		if errorMessage != "" {
			finalText = "Error: " + errorMessage
		} else if strings.TrimSpace(state.DisplayText) != "" {
			finalText = state.DisplayText
		} else {
			finalText = "(no output)"
		}
	}

	delete(a.runs, runID)
	return finalText
}

// Drop removes tracking for a run without finalizing.
func (a *Assembler) Drop(runID string) {
	delete(a.runs, runID)
}

// Reset clears all tracked runs.
func (a *Assembler) Reset() {
	a.runs = make(map[string]*runState)
}

// GetThinking returns the accumulated thinking text for a run, if any.
func (a *Assembler) GetThinking(runID string) string {
	if s, ok := a.runs[runID]; ok {
		return s.ThinkingText
	}
	return ""
}

func (a *Assembler) getOrCreate(runID string) *runState {
	if s, ok := a.runs[runID]; ok {
		return s
	}
	s := &runState{}
	a.runs[runID] = s
	return s
}

func compose(thinking, content string, showThinking bool) string {
	parts := []string{}
	if showThinking && strings.TrimSpace(thinking) != "" {
		parts = append(parts, "[thinking]\n"+thinking)
	}
	if strings.TrimSpace(content) != "" {
		parts = append(parts, content)
	}
	return strings.Join(parts, "\n\n")
}

// extractFromMessage pulls thinking and content text from a message record.
func extractFromMessage(raw json.RawMessage) (thinking, content string) {
	if raw == nil {
		return "", ""
	}

	// Try parsing as a message record
	var record struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return "", ""
	}

	// Content might be a string
	var str string
	if err := json.Unmarshal(record.Content, &str); err == nil {
		return "", strings.TrimSpace(str)
	}

	// Content is an array of blocks
	var blocks []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	}
	if err := json.Unmarshal(record.Content, &blocks); err != nil {
		return "", ""
	}

	var thinkParts, textParts []string
	for _, b := range blocks {
		switch b.Type {
		case "thinking":
			if strings.TrimSpace(b.Thinking) != "" {
				thinkParts = append(thinkParts, b.Thinking)
			}
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				textParts = append(textParts, b.Text)
			}
		}
	}

	return strings.Join(thinkParts, "\n"), strings.Join(textParts, "\n")
}
