package store

import (
	"context"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store/db"
)

func (s *Store) CreateProject(ctx context.Context, p domain.Project) error {
	return s.q.InsertProject(ctx, db.InsertProjectParams{
		ID:            p.ID,
		Name:          p.Name,
		RemoteUrl:     p.RemoteURL,
		DefaultBranch: p.DefaultBranch,
		SetupScript:   p.SetupScript,
		RunScript:     p.RunScript,
		SandboxImage:  p.SandboxImage,
		MergeMode:     p.MergeMode,
		CodeRemote:    p.CodeRemote,
		CreatedAt:     p.CreatedAt.UnixMilli(),
	})
}

func (s *Store) Project(ctx context.Context, id string) (domain.Project, error) {
	row, err := s.q.GetProject(ctx, id)
	if err != nil {
		return domain.Project{}, notFound(err, "project", id)
	}

	return projectFromRow(row), nil
}

func (s *Store) Projects(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.q.ListProjects(ctx)

	return mapRows(rows, err, projectFromRow)
}

func projectFromRow(row db.Project) domain.Project {
	return domain.Project{
		ID:            row.ID,
		Name:          row.Name,
		RemoteURL:     row.RemoteUrl,
		DefaultBranch: row.DefaultBranch,
		SetupScript:   row.SetupScript,
		RunScript:     row.RunScript,
		SandboxImage:  row.SandboxImage,
		MergeMode:     row.MergeMode,
		CodeRemote:    row.CodeRemote,
		CreatedAt:     fromMillis(row.CreatedAt),
	}
}
