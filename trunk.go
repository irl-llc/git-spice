package main

import (
	"context"

	"github.com/irl-llc/git-spice/internal/handler/checkout"
	"github.com/irl-llc/git-spice/internal/spice/state"
)

type trunkCmd struct {
	checkout.Options
}

func (cmd *trunkCmd) Run(
	ctx context.Context,
	store *state.Store,
	checkoutHandler CheckoutHandler,
) error {
	trunk := store.Trunk()
	return checkoutHandler.CheckoutBranch(ctx, &checkout.Request{
		Branch:  trunk,
		Options: &cmd.Options,
	})
}
