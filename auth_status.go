package main

import (
	"errors"
	"fmt"

	"github.com/irl-llc/git-spice/internal/forge"
	"github.com/irl-llc/git-spice/internal/secret"
	"github.com/irl-llc/git-spice/internal/silog"
)

type authStatusCmd struct{}

func (*authStatusCmd) Help() string {
	return `Exits with a non-zero code if not logged in.`
}

func (cmd *authStatusCmd) Run(
	stash secret.Stash,
	log *silog.Logger,
	f forge.Forge,
) error {
	if _, err := f.LoadAuthenticationToken(stash); err != nil {
		if errors.Is(err, secret.ErrNotFound) {
			return fmt.Errorf("%s: not logged in", f.ID())
		}
		return fmt.Errorf("load authentication token: %w", err)
	}

	log.Infof("%s: currently logged in", f.ID())
	return nil
}
