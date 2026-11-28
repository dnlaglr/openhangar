package sandbox

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
)

type Spec struct {
	WorkspaceID string
	RepoDir     string
	Branch      string
}

func NewSpec(w domain.Workspace, repoDir string) Spec {
	return Spec{WorkspaceID: w.ID, RepoDir: repoDir, Branch: w.Branch()}
}

type Cmd struct {
	Name string
	Args []string
	Env  []string
}

type Process interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
	Wait() error
	// Kill stops the process and everything it started, escalating from a graceful signal after a grace period.
	Kill() error
}

type Runtime interface {
	Create(ctx context.Context, spec Spec) error
	Start(ctx context.Context, spec Spec) error
	Stop(ctx context.Context, spec Spec) error
	Destroy(ctx context.Context, spec Spec) error
	Exec(ctx context.Context, spec Spec, cmd Cmd) (Process, error)
	Isolated() bool
}

func Output(ctx context.Context, rt Runtime, spec Spec, cmd Cmd) (string, error) {
	p, err := rt.Exec(ctx, spec, cmd)
	if err != nil {
		return "", err
	}

	closeErr := p.Stdin().Close()
	out, readErr := io.ReadAll(p.Stdout())
	if err := errors.Join(p.Wait(), readErr, closeErr); err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}
