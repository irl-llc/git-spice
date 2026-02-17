package gitlab

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"go.abhg.dev/gs/internal/forge"
)

// MergeChange merges an open merge request into its base branch.
func (r *Repository) MergeChange(
	ctx context.Context, fid forge.ChangeID,
) error {
	id := mustMR(fid)

	_, _, err := r.client.MergeRequests.AcceptMergeRequest(
		r.repoID,
		id.Number,
		&gitlab.AcceptMergeRequestOptions{},
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("merge merge request: %w", err)
	}

	r.log.Debug("Merged merge request", "mr", id.Number)
	return nil
}
