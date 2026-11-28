package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

type Repos struct {
	Root string
}

func (r Repos) Dir(projectID string) string {
	return filepath.Join(r.Root, projectID+".git")
}

func (r Repos) Clone(ctx context.Context, projectID, remoteURL string) (defaultBranch string, err error) {
	dir := r.Dir(projectID)
	if err := os.MkdirAll(r.Root, 0o755); err != nil {
		return "", err
	}
	if _, err := Run(ctx, r.Root, "clone", "--bare", "--", remoteURL, dir); err != nil {
		return "", err
	}

	return Run(ctx, dir, "symbolic-ref", "--short", "HEAD")
}

func (r Repos) CreateBranch(ctx context.Context, projectID, branch, from string) error {
	_, err := Run(ctx, r.Dir(projectID), "branch", "--no-track", "--", branch, from)

	return err
}

func (r Repos) DeleteBranch(ctx context.Context, projectID, branch string) error {
	_, err := Run(ctx, r.Dir(projectID), "branch", "-D", "--", branch)

	return err
}
