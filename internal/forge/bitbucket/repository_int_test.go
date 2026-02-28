package bitbucket

import (
	"context"
	"fmt"
)

// MergeChange merges a pull request.
// This is used by integration tests to put PRs in different states.
func MergeChange(ctx context.Context, repo *Repository, id *PR) error {
	if err := approvePR(ctx, repo, id); err != nil {
		repo.log.Debug("Approval failed (may not be required)", "err", err)
	}

	mergePath := fmt.Sprintf(
		"/repositories/%s/%s/pullrequests/%d/merge",
		repo.workspace, repo.repo, id.Number,
	)
	if err := repo.client.post(ctx, mergePath, nil, nil); err != nil {
		return fmt.Errorf("merge PR: %w", err)
	}
	repo.log.Debug("Merged pull request", "pr", id.Number)
	return nil
}

func approvePR(ctx context.Context, repo *Repository, id *PR) error {
	path := fmt.Sprintf(
		"/repositories/%s/%s/pullrequests/%d/approve",
		repo.workspace, repo.repo, id.Number,
	)
	return repo.client.post(ctx, path, nil, nil)
}

// CloseChange declines (closes) a pull request without merging.
// This is used by integration tests to put PRs in different states.
func CloseChange(ctx context.Context, repo *Repository, id *PR) error {
	path := fmt.Sprintf(
		"/repositories/%s/%s/pullrequests/%d/decline",
		repo.workspace, repo.repo, id.Number,
	)
	if err := repo.client.post(ctx, path, nil, nil); err != nil {
		return fmt.Errorf("decline PR: %w", err)
	}
	repo.log.Debug("Declined pull request", "pr", id.Number)
	return nil
}
