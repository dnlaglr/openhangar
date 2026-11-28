package project

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store"
)

type Service struct {
	Store *store.Store
	Repos git.Repos
}

func (s Service) Add(ctx context.Context, p domain.Project) (domain.Project, error) {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.RemoteURL) == "" {
		return domain.Project{}, fmt.Errorf("project needs a name and a remote url: %w", domain.ErrInvalid)
	}

	p.MergeMode = cmp.Or(p.MergeMode, domain.MergePR)
	p.CodeRemote = cmp.Or(p.CodeRemote, domain.CodeRemoteProject)
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
