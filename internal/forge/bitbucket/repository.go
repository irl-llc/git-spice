package bitbucket

import (
	"context"

	"go.abhg.dev/gs/internal/forge"
	"go.abhg.dev/gs/internal/silog"
)

// Repository is a Bitbucket repository.
type Repository struct {
	client *client

	workspace, repo string
	log             *silog.Logger
	forge           *Forge
}

var _ forge.Repository = (*Repository)(nil)

func newRepository(
	forge *Forge,
	workspace, repo string,
	log *silog.Logger,
	client *client,
) *Repository {
	return &Repository{
		client:    client,
		workspace: workspace,
		repo:      repo,
		forge:     forge,
		log:       log,
	}
}

// Forge returns the forge this repository belongs to.
func (r *Repository) Forge() forge.Forge { return r.forge }

// NewChangeMetadata returns the metadata for a pull request.
func (r *Repository) NewChangeMetadata(
	_ context.Context,
	id forge.ChangeID,
) (forge.ChangeMetadata, error) {
	pr := mustPR(id)
	return &PRMetadata{PR: pr}, nil
}

// ListChangeTemplates lists pull request templates in the repository.
// Bitbucket has limited template support, so this returns an empty list.
func (r *Repository) ListChangeTemplates(
	_ context.Context,
) ([]*forge.ChangeTemplate, error) {
	return nil, nil
}
