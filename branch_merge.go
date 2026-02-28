package main

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/gs/internal/git"
	"go.abhg.dev/gs/internal/handler/merge"
	"go.abhg.dev/gs/internal/spice/state"
	"go.abhg.dev/gs/internal/text"
)

type branchMergeCmd struct {
	Branch string `placeholder:"NAME" help:"Branch to merge" predictor:"trackedBranches"`
	NoWait bool   `help:"Skip waiting for each merge to propagate before proceeding."`
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
	mergeHandler MergeHandler,
) error {
	if cmd.Branch == "" {
		currentBranch, err := wt.CurrentBranch(ctx)
		if err != nil {
			return fmt.Errorf("get current branch: %w", err)
		}
		cmd.Branch = currentBranch
	}

	if cmd.Branch == store.Trunk() {
		return errors.New("cannot merge trunk")
	}

	return mergeHandler.MergeDownstack(ctx, &merge.Request{
		Branch: cmd.Branch,
		NoWait: cmd.NoWait,
	})
}
