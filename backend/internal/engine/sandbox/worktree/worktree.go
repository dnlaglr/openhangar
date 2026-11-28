package worktree

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox"
)

type Runtime struct {
	Root string
}

var _ sandbox.Runtime = Runtime{}

func (r Runtime) Dir(workspaceID string) string {
	return filepath.Join(r.Root, workspaceID)
}

func (r Runtime) Create(ctx context.Context, spec sandbox.Spec) error {
	if err := os.MkdirAll(r.Root, 0o755); err != nil {
		return err
	}

	_, err := git.Run(ctx, spec.RepoDir, "worktree", "add", "--", r.Dir(spec.WorkspaceID), spec.Branch)

	return err
}

func (r Runtime) Start(context.Context, sandbox.Spec) error { return nil }

func (r Runtime) Stop(context.Context, sandbox.Spec) error { return nil }

func (r Runtime) Destroy(ctx context.Context, spec sandbox.Spec) error {
	dir := r.Dir(spec.WorkspaceID)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		_, err := git.Run(ctx, spec.RepoDir, "worktree", "prune")

		return err
	}

	_, err := git.Run(ctx, spec.RepoDir, "worktree", "remove", "--force", "--", dir)

	return err
}
