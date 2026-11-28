package claudecode

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/provider"
)

// The fixtures are real claude 2.1.332 output, trimmed of local paths and plugin lists. Re-record them when the format changes.
func TestParseRecordedStreams(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		want    []string
		wantErr string
	}{
		{fixture: "text", want: []string{"agent_text hello"}},
		{fixture: "tools", want: []string{
			"tool_call Read",
			"tool_result Read",
			"tool_call Edit",
			"tool_call Bash",
			"tool_result Edit",
			"tool_result Bash error",
			"agent_text",
		}},
		{fixture: "interrupt", want: []string{"tool_call Bash", "tool_result Bash error"}, wantErr: "claude: "},
		{fixture: "badresume", wantErr: "No conversation found with session ID"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			res, payloads, err := parseFixture(t, tc.fixture)
			if got := summarize(payloads); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("payloads = %q, want %q", got, tc.want)
			}
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			if res.SessionID == "" {
				t.Fatal("session id not captured")
			}
		})
	}
}

func TestParseResultUsage(t *testing.T) {
	res, _, err := parseFixture(t, "text")
	if err != nil {
		t.Fatal(err)
	}

	want := provider.Result{
		SessionID:  "70cd6f62-91e7-4f59-afa2-90868adfe66a",
		DurationMS: 1626,
		Usage:      domain.Usage{InputTokens: 2, OutputTokens: 4, CacheReadTokens: 9446, CacheWriteTokens: 18780},
		CostUSD:    0.003852660000000001,
	}
	if res != want {
		t.Fatalf("result = %+v, want %+v", res, want)
	}
}

func TestParseWithoutResultFails(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "text.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.SplitAfter(strings.TrimSpace(string(b)), "\n")
	truncated := strings.Join(lines[:len(lines)-1], "")
	_, ended, err := parse(strings.NewReader(truncated), io.Discard, "turn-1", func(domain.Payload) error { return nil })
	if ended || err != errNoResult {
		t.Fatalf("ended = %v, err = %v; want errNoResult", ended, err)
	}
}

func parseFixture(t *testing.T, name string) (provider.Result, []domain.Payload, error) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var payloads []domain.Payload
	res, ended, err := parse(f, io.Discard, "turn-1", func(p domain.Payload) error {
		payloads = append(payloads, p)
		return nil
	})
	if !ended {
		t.Fatalf("stream ended without a result: %v", err)
	}

	return res, payloads, err
}

func summarize(payloads []domain.Payload) []string {
	var lines []string
	tools := map[string]string{}
	for _, p := range payloads {
		switch p := p.(type) {
		case domain.AgentText:
			line := "agent_text"
			if len(p.Text) < 10 {
				line += " " + p.Text
			}
			lines = append(lines, line)
		case domain.ToolCall:
			tools[p.CallID] = p.Name
			lines = append(lines, "tool_call "+p.Name)
		case domain.ToolResult:
			line := "tool_result " + tools[p.CallID]
			if p.IsError {
				line += " error"
			}
			lines = append(lines, line)
		}
	}

	return lines
}
