package chat

import (
	"context"
	"strings"

	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox"
)

const maxSubjectLen = 72

// checkpoint commits whatever the turn left in the sandbox and returns the commit, or "" when nothing changed.
// Hooks are skipped because a checkpoint is a snapshot, not a reviewed commit.
func (s *Service) checkpoint(ctx context.Context, spec sandbox.Spec, prompt string) (string, error) {
	status, err := s.git(ctx, spec, "status", "--porcelain")
	if err != nil || status == "" {
		return "", err
	}

	if _, err := s.git(ctx, spec, "add", "--all"); err != nil {
		return "", err
	}
	if _, err := s.git(ctx, spec, "commit", "--quiet", "--no-verify", "--message", subject(prompt)); err != nil {
		return "", err
	}

	return s.git(ctx, spec, "rev-parse", "HEAD")
}

func (s *Service) git(ctx context.Context, spec sandbox.Spec, args ...string) (string, error) {
	cmd := sandbox.Cmd{Name: "git", Args: args, Env: []string{"GIT_TERMINAL_PROMPT=0"}}

	return sandbox.Output(ctx, s.Runtime, spec, cmd)
}

func subject(prompt string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(prompt), "\n")
	line = strings.TrimSpace(line)
	if r := []rune(line); len(r) > maxSubjectLen {
		return string(r[:maxSubjectLen-1]) + "…"
	}

	return line
}
