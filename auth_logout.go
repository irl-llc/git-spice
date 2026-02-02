package main

import (
	"github.com/irl-llc/git-spice/internal/forge"
	"github.com/irl-llc/git-spice/internal/secret"
	"github.com/irl-llc/git-spice/internal/silog"
	"github.com/irl-llc/git-spice/internal/text"
)

type authLogoutCmd struct{}

func (*authLogoutCmd) Help() string {
	return text.Dedent(`
		The stored authentication information is deleted.
		Use 'gs auth login' to log in again.

		Does not do anything if not logged in.
	`)
}

func (cmd *authLogoutCmd) Run(
	stash secret.Stash,
	log *silog.Logger,
	f forge.Forge,
) error {
	if err := f.ClearAuthenticationToken(stash); err != nil {
		return err
	}

	// TOOD: Forges should present friendly names in addition to IDs.
	log.Infof("%s: logged out", f.ID())
	return nil
}
