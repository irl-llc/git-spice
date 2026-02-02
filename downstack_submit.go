package main

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/irl-llc/git-spice/internal/git"
	"github.com/irl-llc/git-spice/internal/handler/submit"
	"github.com/irl-llc/git-spice/internal/must"
	"github.com/irl-llc/git-spice/internal/spice"
	"github.com/irl-llc/git-spice/internal/spice/state"
	"github.com/irl-llc/git-spice/internal/text"
)

type downstackSubmitCmd struct {
	submitOptions
	submit.BatchOptions

	Branch string `placeholder:"NAME" help:"Branch to start at" predictor:"trackedBranches"`
}

func (*downstackSubmitCmd) Help() string {
	return text.Dedent(`
		Change Requests are created or updated
		for the current branch and all branches below it until trunk.
		Use --branch to start at a different branch.
	`) + "\n" + _submitHelp
}

func (cmd *downstackSubmitCmd) Run(
	ctx context.Context,
	wt *git.Worktree,
	store *state.Store,
	svc *spice.Service,
	submitHandler SubmitHandler,
) error {
	if cmd.Branch == "" {
		currentBranch, err := wt.CurrentBranch(ctx)
		if err != nil {
			return fmt.Errorf("get current branch: %w", err)
		}
		cmd.Branch = currentBranch
	}

	if cmd.Branch == store.Trunk() {
		return errors.New("nothing to submit below trunk")
	}

	downstacks, err := svc.ListDownstack(ctx, cmd.Branch)
	if err != nil {
		return fmt.Errorf("list downstack: %w", err)
	}
	must.NotBeEmptyf(downstacks, "downstack cannot be empty")
	slices.Reverse(downstacks)

	// TODO: separate preparation of the stack from submission

	return submitHandler.SubmitBatch(ctx, &submit.BatchRequest{
		Branches:     downstacks,
		Options:      &cmd.Options,
		BatchOptions: &cmd.BatchOptions,
	})
}
