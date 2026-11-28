package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
