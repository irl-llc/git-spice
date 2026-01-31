package github

import (
	"context"
	"fmt"

	"github.com/shurcooL/githubv4"
	"go.abhg.dev/gs/internal/forge"
)

// CommentCountsByChange retrieves comment resolution counts for multiple PRs.
func (r *Repository) CommentCountsByChange(
	ctx context.Context,
	ids []forge.ChangeID,
) ([]*forge.CommentCounts, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	gqlIDs, err := r.resolveGraphQLIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	threads, err := r.queryReviewThreads(ctx, gqlIDs)
	if err != nil {
		return nil, err
	}

	return r.countReviewThreads(threads, len(ids)), nil
}

func (r *Repository) resolveGraphQLIDs(
	ctx context.Context,
	ids []forge.ChangeID,
) ([]githubv4.ID, error) {
	gqlIDs := make([]githubv4.ID, len(ids))
	for i, id := range ids {
		pr := mustPR(id)
		var err error
		gqlIDs[i], err = r.graphQLID(ctx, pr)
		if err != nil {
			return nil, fmt.Errorf("resolve ID %v: %w", id, err)
		}
	}
	return gqlIDs, nil
}

type reviewThreadNode struct {
	PullRequest struct {
		ReviewThreads struct {
			TotalCount int
			Nodes      []struct {
				IsResolved bool
			}
		} `graphql:"reviewThreads(first: 100)"`
	} `graphql:"... on PullRequest"`
}

func (r *Repository) queryReviewThreads(
	ctx context.Context,
	gqlIDs []githubv4.ID,
) ([]reviewThreadNode, error) {
	var q struct {
		Nodes []reviewThreadNode `graphql:"nodes(ids: $ids)"`
	}

	err := r.client.Query(ctx, &q, map[string]any{"ids": gqlIDs})
	if err != nil {
		return nil, fmt.Errorf("query review threads: %w", err)
	}

	return q.Nodes, nil
}

func (r *Repository) countReviewThreads(
	nodes []reviewThreadNode,
	count int,
) []*forge.CommentCounts {
	results := make([]*forge.CommentCounts, count)
	for i, node := range nodes {
		threads := node.PullRequest.ReviewThreads
		resolved := countResolved(threads.Nodes)
		results[i] = &forge.CommentCounts{
			Total:      threads.TotalCount,
			Resolved:   resolved,
			Unresolved: threads.TotalCount - resolved,
		}
	}
	return results
}

func countResolved(nodes []struct{ IsResolved bool }) int {
	count := 0
	for _, n := range nodes {
		if n.IsResolved {
			count++
		}
	}
	return count
}
