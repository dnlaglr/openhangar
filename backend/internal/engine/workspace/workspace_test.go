package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/gittest"
	"github.com/dnlaglr/openhangar/backend/internal/engine/project"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox/worktree"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store"
)

type env struct {
	svc     Service
	project domain.Project
	rt      worktree.Runtime
}

func newEnv(t *testing.T, wrap func(sandbox.Runtime) sandbox.Runtime) env {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(t.Context(), filepath.Join(root, "engine.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { st.Close() })

	repos := git.Repos{Root: filepath.Join(root, "repos")}

	p, err := project.Service{Store: st, Repos: repos}.Add(t.Context(), domain.Project{
		Name:       "hangar",
		RemoteURL:  gittest.NewRemote(t),
		MergeMode:  domain.MergePR,
		CodeRemote: domain.CodeRemoteProject,
	})
	if err != nil {
		t.Fatal(err)
	}

	rt := worktree.Runtime{Root: filepath.Join(root, "worktrees")}
	var runtime sandbox.Runtime = rt
	if wrap != nil {
		runtime = wrap(rt)
	}

	return env{svc: Service{Store: st, Repos: repos, Runtime: runtime}, project: p, rt: rt}
}

func (e env) hasBranch(t *testing.T, branch string) bool {
	t.Helper()

	return gittest.Must(t, e.svc.Repos.Dir(e.project.ID), "branch", "--list", branch) != ""
}

func TestLifecycle(t *testing.T) {
	e := newEnv(t, nil)
	ctx := t.Context()

	w, chat, err := e.svc.Create(ctx, e.project.ID, "Fix Login!")
	if err != nil {
		t.Fatal(err)
	}
	if w.Status != domain.WorkspaceActive || w.Branch() != "hangar/fix-login" {
		t.Fatalf("created workspace = %+v", w)
	}
	if e.project.DefaultBranch != "main" || !e.hasBranch(t, w.Branch()) {
		t.Fatalf("branch %s missing from bare clone (default %q)", w.Branch(), e.project.DefaultBranch)
	}

	chats, err := e.svc.Store.Chats(ctx, w.ID)
	if err != nil || len(chats) != 1 || chats[0].ID != chat.ID {
		t.Fatalf("chats = %+v, %v; want first chat %s", chats, err, chat.ID)
	}
	if _, err := e.svc.Archive(ctx, w.ID); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("archive while active err = %v, want ErrInvalidTransition", err)
	}

	for _, step := range []struct {
		do   func(context.Context, string) (domain.Workspace, error)
		want domain.WorkspaceStatus
	}{
		{e.svc.Idle, domain.WorkspaceIdle},
		{e.svc.Activate, domain.WorkspaceActive},
		{e.svc.Idle, domain.WorkspaceIdle},
		{e.svc.Archive, domain.WorkspaceArchived},
	} {
		if _, err := step.do(ctx, w.ID); err != nil {
			t.Fatalf("-> %s: %v", step.want, err)
		}

		got, err := e.svc.Store.Workspace(ctx, w.ID)
		if err != nil || got.Status != step.want {
			t.Fatalf("status = %s, %v; want %s", got.Status, err, step.want)
		}
	}
	if _, err := os.Stat(e.rt.Dir(w.ID)); !os.IsNotExist(err) {
		t.Fatalf("archived worktree still on disk: %v", err)
	}
	if !e.hasBranch(t, w.Branch()) {
		t.Fatal("archive must keep the branch")
	}

	again, _, err := e.svc.Create(ctx, e.project.ID, "fix login")
	if err != nil || again.Slug != "fix-login-2" {
		t.Fatalf("second workspace slug = %q, %v; archived slugs stay taken", again.Slug, err)
	}
}

type failingStart struct{ sandbox.Runtime }

func (failingStart) Start(context.Context, sandbox.Spec) error { return errors.New("boom") }

func TestCreateFailureCleansUp(t *testing.T) {
	e := newEnv(t, func(rt sandbox.Runtime) sandbox.Runtime { return failingStart{rt} })
	ctx := t.Context()

	if _, _, err := e.svc.Create(ctx, e.project.ID, "doomed"); err == nil {
		t.Fatal("create succeeded with a failing runtime")
	}
	if slugs, err := e.svc.Store.WorkspaceSlugs(ctx, e.project.ID); err != nil || len(slugs) != 0 {
		t.Fatalf("workspace rows left behind: %v, %v", slugs, err)
	}
	if e.hasBranch(t, "hangar/doomed") {
		t.Fatal("branch left behind")
	}

	entries, _ := os.ReadDir(e.rt.Root)
	if len(entries) != 0 {
		t.Fatalf("worktrees left behind: %v", entries)
	}
}
