package main

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/gs/internal/forge"
	"go.abhg.dev/gs/internal/git"
	"go.abhg.dev/gs/internal/handler/merge"
	"go.abhg.dev/gs/internal/spice"
	"go.abhg.dev/gs/internal/spice/state"
	"go.abhg.dev/gs/internal/text"
)

type branchMergeCmd struct {
	Branch        string `placeholder:"NAME" help:"Branch to merge" predictor:"trackedBranches"`
	NoWait        bool   `help:"Skip waiting for each merge to propagate before proceeding."`
	NoBranchCheck bool   `help:"Skip stale base validation before merging."`
}

func (*branchMergeCmd) Help() string {
	return text.Dedent(`
		Merges the current branch and all branches below it
		into trunk via the forge API, bottom-up.
		Use --branch to start at a different branch.

		Already-merged branches are skipped automatically.
		Branches must have an open Change Request to be merged.
		Between merges, the command waits for each merge
		to complete and retargets the next PR to trunk.
		Use --no-wait to skip this and merge without waiting.

		Before merging, the downstack is checked for branches
		whose base PR was already merged on the forge.
		Use --no-branch-check to skip this validation.

		After merging, run 'gs repo sync' to update local state.
	`)
}

// MergeHandler merges change requests via a forge.
type MergeHandler interface {
	MergeDownstack(ctx context.Context, req *merge.Request) error
}

func (cmd *branchMergeCmd) Run(
	ctx context.Context,
	wt *git.Worktree,
	store *state.Store,
	svc *spice.Service,
	forgeRepo forge.Repository,
	mergeHandler MergeHandler,
) error {
	branch, err := cmd.resolveBranch(ctx, wt)
	if err != nil {
		return err
	}

	if branch == store.Trunk() {
		return errors.New("cannot merge trunk")
	}

	if err := cmd.checkDownstack(
		ctx, svc, forgeRepo, branch,
	); err != nil {
		return err
	}

	return mergeHandler.MergeDownstack(ctx, &merge.Request{
		Branch: branch,
		NoWait: cmd.NoWait,
	})
}

func (cmd *branchMergeCmd) resolveBranch(
	ctx context.Context, wt *git.Worktree,
) (string, error) {
	if cmd.Branch != "" {
		return cmd.Branch, nil
	}
	branch, err := wt.CurrentBranch(ctx)
	if err != nil {
		return "", fmt.Errorf("get current branch: %w", err)
	}
	return branch, nil
}

func (cmd *branchMergeCmd) checkDownstack(
	ctx context.Context,
	svc *spice.Service,
	forgeRepo forge.Repository,
	branch string,
) error {
	if cmd.NoBranchCheck {
		return nil
	}

	graph, err := svc.BranchGraph(ctx, nil)
	if err != nil {
		return fmt.Errorf("build branch graph: %w", err)
	}

	err = spice.ValidateDownstack(ctx, graph, forgeRepo, branch)
	var staleErr *spice.StaleBaseError
	if errors.As(err, &staleErr) {
		return fmt.Errorf(
			"%s has stale base %s (already merged); "+
				"run 'gs repo sync' first, "+
				"or use --no-branch-check to bypass",
			staleErr.Branch, staleErr.Base,
		)
	}
	return err
}
