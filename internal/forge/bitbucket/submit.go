package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.abhg.dev/gs/internal/forge"
)

// SubmitChange creates a new pull request in the repository.
func (r *Repository) SubmitChange(
	ctx context.Context,
	req forge.SubmitChangeRequest,
) (forge.SubmitChangeResult, error) {
	r.warnUnsupportedFeatures(req)

	reviewers, err := r.resolveReviewerUUIDs(ctx, req.Reviewers)
	if err != nil {
		return forge.SubmitChangeResult{}, fmt.Errorf("resolve reviewers: %w", err)
	}

	apiReq := r.buildCreatePRRequest(req, reviewers)
	pr, err := r.createPullRequest(ctx, apiReq)
	if err != nil {
		return forge.SubmitChangeResult{}, err
	}

	r.log.Debug("Created pull request", "pr", pr.ID, "url", pr.Links.HTML.Href)
	return forge.SubmitChangeResult{
		ID:  &PR{Number: pr.ID},
		URL: pr.Links.HTML.Href,
	}, nil
}

func (r *Repository) warnUnsupportedFeatures(req forge.SubmitChangeRequest) {
	if len(req.Labels) > 0 {
		r.log.Warn("Bitbucket does not support PR labels; ignoring --label flags")
	}
	if len(req.Assignees) > 0 {
		r.log.Warn("Bitbucket does not support PR assignees; ignoring --assign flags")
	}
}

func (r *Repository) buildCreatePRRequest(
	req forge.SubmitChangeRequest,
	reviewers []apiReviewer,
) *apiCreatePRRequest {
	apiReq := &apiCreatePRRequest{
		Title: req.Subject,
		Source: apiBranchRef{
			Branch: apiBranch{Name: req.Head},
		},
		Destination: apiBranchRef{
			Branch: apiBranch{Name: req.Base},
		},
		Draft: req.Draft,
	}
	if req.Body != "" {
		apiReq.Description = req.Body
	}
	if len(reviewers) > 0 {
		apiReq.Reviewers = reviewers
	}
	return apiReq
}

func (r *Repository) createPullRequest(
	ctx context.Context,
	req *apiCreatePRRequest,
) (*apiPullRequest, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests", r.workspace, r.repo)

	var resp apiPullRequest
	if err := r.client.post(ctx, path, req, &resp); err != nil {
		if isDestinationBranchNotFound(err) {
			return nil, fmt.Errorf("create pull request: %w", forge.ErrUnsubmittedBase)
		}
		return nil, fmt.Errorf("create pull request: %w", err)
	}
	return &resp, nil
}

// isDestinationBranchNotFound checks if the error indicates
// the destination branch doesn't exist.
func isDestinationBranchNotFound(err error) bool {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode != 400 {
		return false
	}
	return strings.Contains(apiErr.Body, "destination") &&
		strings.Contains(apiErr.Body, "branch not found")
}

func (r *Repository) resolveReviewerUUIDs(
	ctx context.Context,
	usernames []string,
) ([]apiReviewer, error) {
	if len(usernames) == 0 {
		return nil, nil
	}

	reviewers := make([]apiReviewer, 0, len(usernames))
	for _, username := range usernames {
		user, err := r.getUser(ctx, username)
		if err != nil {
			return nil, fmt.Errorf("lookup user %q: %w", username, err)
		}
		reviewers = append(reviewers, apiReviewer{UUID: user.UUID})
		r.log.Debug("Resolved reviewer", "username", username, "uuid", user.UUID)
	}
	return reviewers, nil
}

func (r *Repository) getUser(ctx context.Context, username string) (*apiUser, error) {
	user, err := r.findWorkspaceMember(ctx, username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user %q not found in workspace %q", username, r.workspace)
	}
	return user, nil
}

func (r *Repository) findWorkspaceMember(
	ctx context.Context,
	username string,
) (*apiUser, error) {
	path := fmt.Sprintf("/workspaces/%s/members", r.workspace)
	for path != "" {
		user, nextPath, err := r.searchMemberPage(ctx, path, username)
		if err != nil {
			return nil, err
		}
		if user != nil {
			return user, nil
		}
		path = nextPath
	}
	return nil, nil
}

func (r *Repository) searchMemberPage(
	ctx context.Context,
	path, username string,
) (*apiUser, string, error) {
	var resp apiWorkspaceMemberList
	if err := r.client.get(ctx, path, &resp); err != nil {
		return nil, "", fmt.Errorf("list workspace members: %w", err)
	}

	for _, member := range resp.Values {
		if matchesUsername(&member.User, username) {
			return &member.User, "", nil
		}
	}
	return nil, resp.Next, nil
}

// matchesUsername checks if the user matches the given username.
// It checks Username first (for backward compatibility), then Nickname
// (since Bitbucket deprecated usernames in favor of account IDs).
func matchesUsername(user *apiUser, username string) bool {
	if user.Username != "" && strings.EqualFold(user.Username, username) {
		return true
	}
	return strings.EqualFold(user.Nickname, username)
}
