package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store"
)

const maxSlugLen = 48

type Service struct {
	Store   *store.Store
	Repos   git.Repos
	Runtime sandbox.Runtime
}

func (s Service) Create(ctx context.Context, projectID, name string, agent domain.Agent) (domain.Workspace, domain.Chat, error) {
	p, err := s.Store.Project(ctx, projectID)
	if err != nil {
		return domain.Workspace{}, domain.Chat{}, err
	}

	taken, err := s.Store.WorkspaceSlugs(ctx, projectID)
	if err != nil {
		return domain.Workspace{}, domain.Chat{}, err
	}

	now := domain.Now()
	w := domain.Workspace{
		ID:        domain.NewID(),
		ProjectID: projectID,
		Name:      name,
		Slug:      uniqueSlug(slugify(name), taken),
		Status:    domain.WorkspaceCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}

	chat := domain.Chat{ID: domain.NewID(), WorkspaceID: w.ID, Title: name, Agent: agent, CreatedAt: now}
	if err := s.provision(ctx, p, w, chat); err != nil {
		return domain.Workspace{}, domain.Chat{}, err
	}

	w.Status = domain.WorkspaceActive

	return w, chat, nil
}

func (s Service) provision(ctx context.Context, p domain.Project, w domain.Workspace, chat domain.Chat) (err error) {
	var undo []func(context.Context) error
	defer func() {
		if err == nil {
			return
		}
		cleanupCtx := context.WithoutCancel(ctx)
		for _, fn := range slices.Backward(undo) {
			err = errors.Join(err, fn(cleanupCtx))
		}
	}()
	spec := s.spec(w)

	if err = s.Store.CreateWorkspace(ctx, w); err != nil {
		return err
	}
	undo = append(undo, func(ctx context.Context) error { return s.Store.DeleteWorkspace(ctx, w.ID) })

	if err = s.Repos.CreateBranch(ctx, p.ID, w.Branch(), p.DefaultBranch); err != nil {
		return err
	}
	undo = append(undo, func(ctx context.Context) error { return s.Repos.DeleteBranch(ctx, p.ID, w.Branch()) })

	if err = s.Runtime.Create(ctx, spec); err != nil {
		return err
	}
	undo = append(undo, func(ctx context.Context) error { return s.Runtime.Destroy(ctx, spec) })

	if err = s.Runtime.Start(ctx, spec); err != nil {
		return err
	}

	return s.Store.InTx(ctx, func(tx *store.Store) error {
		if err := tx.SetWorkspaceStatus(ctx, w.ID, domain.WorkspaceCreated, domain.WorkspaceActive, w.UpdatedAt); err != nil {
			return err
		}
		return tx.CreateChat(ctx, chat)
	})
}

// Reconcile removes workspaces a crash left half-created. Run it before serving, never alongside Create.
func (s Service) Reconcile(ctx context.Context) error {
	stale, err := s.Store.WorkspacesWithStatus(ctx, domain.WorkspaceCreated)
	if err != nil {
		return err
	}

	for _, w := range stale {
		slog.WarnContext(ctx, "removing half-created workspace", "workspace_id", w.ID)
		if err := s.Runtime.Destroy(ctx, s.spec(w)); err != nil {
			slog.WarnContext(ctx, "destroy half-created sandbox", "workspace_id", w.ID, "err", err)
		}
		if err := s.Repos.DeleteBranch(ctx, w.ProjectID, w.Branch()); err != nil {
			slog.WarnContext(ctx, "delete half-created branch", "workspace_id", w.ID, "err", err)
		}
		if err := s.Store.DeleteWorkspace(ctx, w.ID); err != nil {
			return err
		}
	}

	return nil
}

func (s Service) Activate(ctx context.Context, id string) (domain.Workspace, error) {
	return s.transition(ctx, id, domain.WorkspaceIdle, domain.WorkspaceActive, s.Runtime.Start)
}

func (s Service) Idle(ctx context.Context, id string) (domain.Workspace, error) {
	return s.transition(ctx, id, domain.WorkspaceActive, domain.WorkspaceIdle, s.Runtime.Stop)
}

func (s Service) Archive(ctx context.Context, id string) (domain.Workspace, error) {
	return s.transition(ctx, id, domain.WorkspaceIdle, domain.WorkspaceArchived, s.Runtime.Destroy)
}

func (s Service) transition(
	ctx context.Context,
	id string,
	from, to domain.WorkspaceStatus,
	effect func(context.Context, sandbox.Spec) error,
) (domain.Workspace, error) {
	w, err := s.Store.Workspace(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}

	if w.Status != from {
		return domain.Workspace{}, fmt.Errorf("workspace %s is %s, want %s: %w", id, w.Status, from, domain.ErrInvalidTransition)
	}

	if err := effect(ctx, s.spec(w)); err != nil {
		return domain.Workspace{}, err
	}

	now := domain.Now()
	if err := s.Store.SetWorkspaceStatus(ctx, id, from, to, now); err != nil {
		return domain.Workspace{}, err
	}

	w.Status, w.UpdatedAt = to, now

	return w, nil
}

func (s Service) spec(w domain.Workspace) sandbox.Spec {
	return sandbox.NewSpec(w, s.Repos.Dir(w.ProjectID))
}

func slugify(name string) string {
	var b strings.Builder
	dash := false

	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}

	slug := strings.TrimRight(b.String()[:min(b.Len(), maxSlugLen)], "-")
	if slug == "" {
		return "workspace"
	}

	return slug
}

func uniqueSlug(base string, taken []string) string {
	slug := base
	for n := 2; slices.Contains(taken, slug); n++ {
		slug = fmt.Sprintf("%s-%d", base, n)
	}

	return slug
}
