package store

import (
	"context"
	"fmt"
	"time"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store/db"
)

func (s *Store) CreateWorkspace(ctx context.Context, w domain.Workspace) error {
	return s.q.InsertWorkspace(ctx, db.InsertWorkspaceParams{
		ID:        w.ID,
		ProjectID: w.ProjectID,
		Name:      w.Name,
		Slug:      w.Slug,
		Status:    w.Status,
		CreatedAt: w.CreatedAt.UnixMilli(),
		UpdatedAt: w.UpdatedAt.UnixMilli(),
	})
}

func (s *Store) Workspace(ctx context.Context, id string) (domain.Workspace, error) {
	row, err := s.q.GetWorkspace(ctx, id)
	if err != nil {
		return domain.Workspace{}, notFound(err, "workspace", id)
	}

	return workspaceFromRow(row), nil
}

func (s *Store) Workspaces(ctx context.Context, projectID string) ([]domain.Workspace, error) {
	rows, err := s.q.ListWorkspaces(ctx, projectID)

	return mapRows(rows, err, workspaceFromRow)
}

func (s *Store) WorkspacesWithStatus(ctx context.Context, status domain.WorkspaceStatus) ([]domain.Workspace, error) {
	rows, err := s.q.ListWorkspacesByStatus(ctx, status)

	return mapRows(rows, err, workspaceFromRow)
}

func workspaceFromRow(row db.Workspace) domain.Workspace {
	return domain.Workspace{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Name:      row.Name,
		Slug:      row.Slug,
		Status:    row.Status,
		CreatedAt: fromMillis(row.CreatedAt),
		UpdatedAt: fromMillis(row.UpdatedAt),
	}
}

func (s *Store) WorkspaceSlugs(ctx context.Context, projectID string) ([]string, error) {
	return s.q.ListWorkspaceSlugs(ctx, projectID)
}

func (s *Store) SetWorkspaceStatus(ctx context.Context, id string, from, to domain.WorkspaceStatus, at time.Time) error {
	if !from.CanTransition(to) {
		return fmt.Errorf("workspace %s %s -> %s: %w", id, from, to, domain.ErrInvalidTransition)
	}

	n, err := s.q.UpdateWorkspaceStatus(ctx, db.UpdateWorkspaceStatusParams{
		ID:         id,
		FromStatus: from,
		ToStatus:   to,
		UpdatedAt:  at.UnixMilli(),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("workspace %s is no longer %s: %w", id, from, domain.ErrInvalidTransition)
	}

	return nil
}

func (s *Store) DeleteWorkspace(ctx context.Context, id string) error {
	return s.q.DeleteWorkspace(ctx, id)
}
