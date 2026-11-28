package worktree

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/gittest"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox"
)

func TestCreateAndDestroy(t *testing.T) {
	ctx := t.Context()
	repos := git.Repos{Root: t.TempDir()}

	if _, err := repos.Clone(ctx, "p1", gittest.NewRemote(t)); err != nil {
		t.Fatal(err)
	}

	if err := repos.CreateBranch(ctx, "p1", "hangar/demo", "main"); err != nil {
		t.Fatal(err)
	}

	rt := Runtime{Root: t.TempDir()}
	spec := sandbox.Spec{WorkspaceID: "w1", RepoDir: repos.Dir("p1"), Branch: "hangar/demo"}
	dir := rt.Dir(spec.WorkspaceID)

	if err := rt.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}

	if branch := gittest.Must(t, dir, "branch", "--show-current"); branch != "hangar/demo" {
		t.Fatalf("worktree on %q, want hangar/demo", branch)
	}

	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("worktree missing checkout: %v", err)
	}

	if err := rt.Destroy(ctx, spec); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still present: %v", err)
	}

	if list := gittest.Must(t, spec.RepoDir, "worktree", "list"); strings.Contains(list, dir) {
		t.Fatalf("worktree still registered:\n%s", list)
	}

	if err := rt.Destroy(ctx, spec); err != nil {
		t.Fatalf("second destroy must be a no-op: %v", err)
	}
}

func TestKillStopsBackgroundChildren(t *testing.T) {
	rt := Runtime{Root: t.TempDir()}
	spec := sandbox.Spec{WorkspaceID: "w1"}
	if err := os.MkdirAll(rt.Dir(spec.WorkspaceID), 0o755); err != nil {
		t.Fatal(err)
	}

	p, err := rt.Exec(t.Context(), spec, sandbox.Cmd{Name: "sh", Args: []string{"-c", "sleep 60 & echo started; wait"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(p.Stdout(), make([]byte, len("started\n"))); err != nil {
		t.Fatal(err)
	}

	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}

	drained := make(chan struct{})
	go func() {
		_, _ = io.ReadAll(p.Stdout())
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(3 * time.Second):
		t.Fatal("background child still holds the pipe after Kill")
	}
	if err := p.Wait(); err == nil {
		t.Fatal("killed process exited cleanly")
	}
}
