package domain

import "encoding/json"

const (
	EventUserMessage = "user_message"
	EventTurnStarted = "turn_started"
	EventAgentText   = "agent_text"
	EventToolCall    = "tool_call"
	EventToolResult  = "tool_result"
	EventTurnEnded   = "turn_ended"
)

const AuthorAgent = "agent"

type Payload interface {
	EventType() string
}

type UserMessage struct {
	Text string `json:"text"`
}

type TurnStarted struct {
	TurnID     string   `json:"turn_id"`
	MessageIDs []string `json:"message_ids"`
	Agent
}

type AgentText struct {
	TurnID string `json:"turn_id"`
	Text   string `json:"text"`
}

type ToolCall struct {
	TurnID       string          `json:"turn_id"`
	CallID       string          `json:"call_id"`
	ParentCallID string          `json:"parent_call_id,omitempty"`
	Name         string          `json:"name"`
	Input        json.RawMessage `json:"input"`
}

type ToolResult struct {
	TurnID  string `json:"turn_id"`
	CallID  string `json:"call_id"`
	Output  string `json:"output"`
	IsError bool   `json:"is_error,omitempty"`
}

type Outcome string

const (
	OutcomeCompleted   Outcome = "completed"
	OutcomeInterrupted Outcome = "interrupted"
	OutcomeFailed      Outcome = "failed"
)

type Usage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

type TurnEnded struct {
	TurnID     string  `json:"turn_id"`
	Outcome    Outcome `json:"outcome"`
	Error      string  `json:"error,omitempty"`
	DurationMS int64   `json:"duration_ms"`
	Usage      Usage   `json:"usage"`
	CostUSD    float64 `json:"cost_usd"`
	Commit     string  `json:"commit,omitempty"`
}

func (UserMessage) EventType() string { return EventUserMessage }
func (TurnStarted) EventType() string { return EventTurnStarted }
func (AgentText) EventType() string   { return EventAgentText }
func (ToolCall) EventType() string    { return EventToolCall }
func (ToolResult) EventType() string  { return EventToolResult }
func (TurnEnded) EventType() string   { return EventTurnEnded }
