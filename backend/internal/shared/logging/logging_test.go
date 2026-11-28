package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggerCorrelatesAndRedacts(t *testing.T) {
	var buf bytes.Buffer
	level := new(slog.LevelVar)
	log := New(&buf, level, NewRedactor("/home/alice", ""))
	ctx := WithTurn(WithChat(WithWorkspace(t.Context(), "w1"), "c1"), "t1")

	log.DebugContext(ctx, "hidden")
	level.Set(slog.LevelDebug)
	log.With("component", "git").ErrorContext(ctx, "clone failed",
		"err", errors.New("git clone https://x-access-token:secret@github.com/acme/repo.git /home/alice/repos: exit 128"))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1:\n%s", len(lines), buf.String())
	}

	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}

	want := map[string]any{
		"workspace_id": "w1",
		"chat_id":      "c1",
		"turn_id":      "t1",
		"component":    "git",
		"err":          "git clone https://<redacted> ~/repos: exit 128",
	}
	for k, v := range want {
		if rec[k] != v {
			t.Errorf("%s = %v, want %v", k, rec[k], v)
		}
	}

	if _, ok := rec["host_id"]; ok {
		t.Error("host_id present without WithHost")
	}
}

func TestRawWritesOnlyAtDebug(t *testing.T) {
	var buf bytes.Buffer
	level := new(slog.LevelVar)
	w := Raw(&buf, level, NewRedactor("/home/alice", ""))

	fmt.Fprintln(w, "prompt before debug mode")
	level.Set(slog.LevelDebug)
	fmt.Fprintln(w, `{"cwd":"/home/alice/src"}`)

	if got, want := buf.String(), `{"cwd":"~/src"}`+"\n"; got != want {
		t.Fatalf("raw output %q, want %q", got, want)
	}
}
