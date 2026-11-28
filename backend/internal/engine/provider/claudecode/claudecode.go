package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/provider"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox"
)

const Name = "claude-code"

const (
	interruptGrace = 10 * time.Second
	exitGrace      = 5 * time.Second
)

type Provider struct {
	Runtime sandbox.Runtime
	Bin     string
	Raw     io.Writer
}

var _ provider.Provider = Provider{}

func (p Provider) Run(ctx context.Context, req provider.Request, emit func(domain.Payload) error) (provider.Result, error) {
	cmd := sandbox.Cmd{
		Name: p.bin(),
		Args: args(req, p.Runtime.Isolated()),
		Env:  []string{"CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1"},
	}
	proc, err := p.Runtime.Exec(context.WithoutCancel(ctx), req.Spec, cmd)
	if err != nil {
		return provider.Result{}, err
	}

	if err := writeLine(proc.Stdin(), newUserMessage(req.Prompts)); err != nil {
		return provider.Result{}, errors.Join(err, proc.Kill(), proc.Wait())
	}

	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		interruptOnCancel(ctx, proc, done)
	}()

	raw := p.Raw
	if raw == nil {
		raw = io.Discard
	}
	res, ended, err := parse(proc.Stdout(), raw, req.TurnID, emit)
	close(done)
	<-stopped

	if !ended {
		_ = proc.Kill()
	}
	waitErr := errors.Join(proc.Stdin().Close(), waitOrKill(proc))
	if !ended {
		return res, errors.Join(err, waitErr)
	}

	return res, err
}

func (p Provider) bin() string {
	if p.Bin == "" {
		return "claude"
	}

	return p.Bin
}

func args(req provider.Request, isolated bool) []string {
	a := []string{
		"-p", "--verbose",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--permission-prompts", "none",
	}
	if isolated {
		a = append(a, "--dangerously-skip-permissions")
	} else {
		a = append(a, "--permission-mode", "auto")
	}
	if req.Model != "" {
		a = append(a, "--model", req.Model)
	}
	if req.Effort != "" {
		a = append(a, "--effort", req.Effort)
	}
	// TODO: claude finds a session by its cwd under ~/.claude/projects; a host handoff must move the session file to the same path.
	if req.SessionID != "" {
		a = append(a, "--resume", req.SessionID)
	}

	return a
}

// waitOrKill gives claude exitGrace to exit after its input closes. A turn ends at its result, so nothing it started may outlive it.
func waitOrKill(proc sandbox.Process) error {
	exited := make(chan error, 1)
	go func() { exited <- proc.Wait() }()

	select {
	case err := <-exited:
		return err
	case <-time.After(exitGrace):
		_ = proc.Kill()

		return <-exited
	}
}

// interruptOnCancel asks claude to stop gracefully so the session stays resumable, and kills it if it doesn't.
func interruptOnCancel(ctx context.Context, proc sandbox.Process, done <-chan struct{}) {
	select {
	case <-done:
		return
	case <-ctx.Done():
	}

	if err := writeLine(proc.Stdin(), interruptRequest); err != nil {
		_ = proc.Kill()

		return
	}

	select {
	case <-done:
	case <-time.After(interruptGrace):
		_ = proc.Kill()
	}
}

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type userMessage struct {
	Type    string `json:"type"`
	Message struct {
		Role    string      `json:"role"`
		Content []textBlock `json:"content"`
	} `json:"message"`
}

func newUserMessage(prompts []string) userMessage {
	m := userMessage{Type: "user"}
	m.Message.Role = "user"
	for _, p := range prompts {
		m.Message.Content = append(m.Message.Content, textBlock{Type: "text", Text: p})
	}

	return m
}

var interruptRequest = map[string]any{
	"type":       "control_request",
	"request_id": "interrupt",
	"request":    map[string]string{"subtype": "interrupt"},
}

func writeLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))

	return err
}
