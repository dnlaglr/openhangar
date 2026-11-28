package claudecode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/provider"
)

var errNoResult = errors.New("claude exited without a result")

type stream struct {
	turnID string
	emit   func(domain.Payload) error
	res    provider.Result
}

// parse maps claude's stream-json output to domain payloads until the result message. ended reports whether it arrived.
func parse(r io.Reader, raw io.Writer, turnID string, emit func(domain.Payload) error) (res provider.Result, ended bool, err error) {
	s := stream{turnID: turnID, emit: emit}
	br := bufio.NewReader(r)
	for {
		line, readErr := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			_, _ = raw.Write(line)
			if ended, err := s.handle(line); ended || err != nil {
				return s.res, ended, err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return s.res, false, errNoResult
		}
		if readErr != nil {
			return s.res, false, readErr
		}
	}
}

func (s *stream) handle(line []byte) (ended bool, err error) {
	var head struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return false, fmt.Errorf("decode claude message: %w", err)
	}
	if head.SessionID != "" {
		s.res.SessionID = head.SessionID
	}

	switch head.Type {
	case "assistant":
		return false, s.assistant(line)
	case "user":
		return false, s.user(line)
	case "result":
		return true, s.result(line)
	}

	return false, nil
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type chatMessage struct {
	ParentToolUseID string `json:"parent_tool_use_id"`
	Message         struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func (s *stream) assistant(line []byte) error {
	m, blocks, err := decodeChatMessage(line)
	if err != nil {
		return err
	}

	for _, b := range blocks {
		var p domain.Payload
		switch {
		case b.Type == "text" && b.Text != "":
			p = domain.AgentText{TurnID: s.turnID, Text: b.Text}
		case b.Type == "tool_use":
			p = domain.ToolCall{TurnID: s.turnID, CallID: b.ID, ParentCallID: m.ParentToolUseID, Name: b.Name, Input: b.Input}
		default:
			continue
		}
		if err := s.emit(p); err != nil {
			return err
		}
	}

	return nil
}

func (s *stream) user(line []byte) error {
	_, blocks, err := decodeChatMessage(line)
	if err != nil {
		return err
	}

	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		p := domain.ToolResult{TurnID: s.turnID, CallID: b.ToolUseID, Output: toolOutput(b.Content), IsError: b.IsError}
		if err := s.emit(p); err != nil {
			return err
		}
	}

	return nil
}

func decodeChatMessage(line []byte) (chatMessage, []contentBlock, error) {
	var m chatMessage
	if err := json.Unmarshal(line, &m); err != nil {
		return m, nil, fmt.Errorf("decode claude message: %w", err)
	}

	var blocks []contentBlock
	if !bytes.HasPrefix(bytes.TrimSpace(m.Message.Content), []byte("[")) {
		return m, nil, nil
	}
	if err := json.Unmarshal(m.Message.Content, &blocks); err != nil {
		return m, nil, fmt.Errorf("decode claude content: %w", err)
	}

	return m, blocks, nil
}

func toolOutput(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}

	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return string(raw)
	}

	parts := make([]string, len(blocks))
	for i, b := range blocks {
		if b.Type == "text" {
			parts[i] = b.Text
		} else {
			parts[i] = "[" + b.Type + "]"
		}
	}

	return strings.Join(parts, "\n")
}

func (s *stream) result(line []byte) error {
	var r struct {
		Subtype      string   `json:"subtype"`
		IsError      bool     `json:"is_error"`
		DurationMS   int64    `json:"duration_ms"`
		TotalCostUSD float64  `json:"total_cost_usd"`
		Errors       []string `json:"errors"`
		Usage        struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(line, &r); err != nil {
		return fmt.Errorf("decode claude result: %w", err)
	}

	s.res.DurationMS = r.DurationMS
	s.res.CostUSD = r.TotalCostUSD
	s.res.Usage = domain.Usage{
		InputTokens:      r.Usage.InputTokens,
		OutputTokens:     r.Usage.OutputTokens,
		CacheReadTokens:  r.Usage.CacheReadInputTokens,
		CacheWriteTokens: r.Usage.CacheCreationInputTokens,
	}
	if !r.IsError {
		return nil
	}
	if len(r.Errors) == 0 {
		return fmt.Errorf("claude: %s", r.Subtype)
	}

	return fmt.Errorf("claude: %s", strings.Join(r.Errors, "; "))
}
