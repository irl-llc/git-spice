package main

import (
	"context"

	"github.com/alecthomas/kong"
	"github.com/irl-llc/git-spice/internal/git"
	"github.com/irl-llc/git-spice/internal/text"
)

type logShortCmd struct {
	branchLogCmd
}

func (*logShortCmd) Help() string {
	return text.Dedent(`
		Only branches that are upstack and downstack from the current
		branch are shown.
		Use with the -a/--all flag to show all tracked branches.

		With --json, prints output to stdout as a stream of JSON objects.
		See https://abhinav.github.io/git-spice/cli/json/ for details.
	`)
}

func (cmd *logShortCmd) Run(
	ctx context.Context,
	kctx *kong.Context,
	wt *git.Worktree,
	listHandler ListHandler,
) (err error) {
	return cmd.run(ctx, kctx, nil, wt, listHandler)
}
