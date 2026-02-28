package gitlab

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// NewRepository re-exports the private NewRepository function
// for testing.
var NewRepository = newRepository

// RepositoryOptions re-exports the private repositoryOptions type
type RepositoryOptions = repositoryOptions

// MergeChange merges a merge request using the production method.
func MergeChange(ctx context.Context, repo *Repository, id *MR) error {
	return repo.MergeChange(ctx, id)
}

func CloseChange(ctx context.Context, repo *Repository, id *MR) error {
	_, _, err := repo.client.MergeRequests.UpdateMergeRequest(
		repo.repoID,
		id.Number,
		&gitlab.UpdateMergeRequestOptions{
			StateEvent: new("close"),
		},
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("close merge request: %w", err)
	}
	repo.log.Debug("Closed merge request", "mr", id.Number)
	return nil
}
