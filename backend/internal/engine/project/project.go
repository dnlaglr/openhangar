package project

import (
	"context"
	"errors"
	"os"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store"
)

type Service struct {
	Store *store.Store
	Repos git.Repos
}

func (s Service) Add(ctx context.Context, p domain.Project) (domain.Project, error) {
	p.ID = domain.NewID()
	p.CreatedAt = domain.Now()
	head, err := s.Repos.Clone(ctx, p.ID, p.RemoteURL)
	if err != nil {
		return domain.Project{}, errors.Join(err, os.RemoveAll(s.Repos.Dir(p.ID)))
	}

	if p.DefaultBranch == "" {
		p.DefaultBranch = head
	}

	if err := s.Store.CreateProject(ctx, p); err != nil {
		return domain.Project{}, errors.Join(err, os.RemoveAll(s.Repos.Dir(p.ID)))
	}

	return p, nil
}
