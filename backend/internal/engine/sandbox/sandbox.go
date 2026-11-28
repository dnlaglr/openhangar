package sandbox

import "context"

type Spec struct {
	WorkspaceID string
	RepoDir     string
	Branch      string
}

type Runtime interface {
	Create(ctx context.Context, spec Spec) error
	Start(ctx context.Context, spec Spec) error
	Stop(ctx context.Context, spec Spec) error
	Destroy(ctx context.Context, spec Spec) error
}
