package main

import (
	"context"
	"fmt"

	"github.com/irl-llc/git-spice/internal/git"
	"github.com/irl-llc/git-spice/internal/handler/autostash"
	"github.com/irl-llc/git-spice/internal/handler/restack"
	"github.com/irl-llc/git-spice/internal/silog"
	"github.com/irl-llc/git-spice/internal/spice/state"
	"github.com/irl-llc/git-spice/internal/text"
)

type repoRestackCmd struct{}

func (*repoRestackCmd) Help() string {
	return text.Dedent(`
		All tracked branches in the repository are rebased on top of their
		respective bases in dependency order, ensuring a linear history.
	`)
}

func (*repoRestackCmd) Run(
	ctx context.Context,
	log *silog.Logger,
	wt *git.Worktree,
	store *state.Store,
	handler RestackHandler,
	autostashHandler AutostashHandler,
) (retErr error) {
	currentBranch, err := wt.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}

	cleanup, err := autostashHandler.BeginAutostash(ctx, &autostash.Options{
		Message:   "git-spice: autostash before restacking",
		ResetMode: autostash.ResetHard,
		Branch:    currentBranch,
	})
	if err != nil {
		return err
	}
	defer cleanup(&retErr)

	count, err := handler.Restack(ctx, &restack.Request{
		Branch:          store.Trunk(),
		Scope:           restack.ScopeUpstackExclusive,
		ContinueCommand: []string{"repo", "restack"},
	})
	if err != nil {
		return err
	}

	if count == 0 {
		log.Infof("Nothing to restack: no tracked branches available")
		return nil
	}

	if err := wt.CheckoutBranch(ctx, currentBranch); err != nil {
		return fmt.Errorf("checkout %v: %w", currentBranch, err)
	}

	log.Infof("Restacked %d branches", count)
	return nil
}
