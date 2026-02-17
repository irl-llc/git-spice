package bitbucket

import (
	"context"
	"fmt"

	"go.abhg.dev/gs/internal/forge"
)

// MergeChange merges an open pull request into its base branch.
func (r *Repository) MergeChange(
	ctx context.Context, fid forge.ChangeID,
) error {
	id := mustPR(fid)

	path := fmt.Sprintf(
		"/repositories/%s/%s/pullrequests/%d/merge",
		r.workspace, r.repo, id.Number,
	)
	if err := r.client.post(ctx, path, nil, nil); err != nil {
		return fmt.Errorf("merge pull request: %w", err)
	}

	r.log.Debug("Merged pull request", "pr", id.Number)
	return nil
}
