// Package merge implements the branch merge command.
package merge

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.abhg.dev/gs/internal/forge"
	"go.abhg.dev/gs/internal/silog"
	"go.abhg.dev/gs/internal/spice"
	"go.abhg.dev/gs/internal/ui"
)

// Store provides read access to the state store.
type Store interface {
	Trunk() string
}

// Service provides branch graph operations.
type Service interface {
	ListDownstack(ctx context.Context, start string) ([]string, error)
	LookupBranch(
		ctx context.Context, name string,
	) (*spice.LookupBranchResponse, error)
}

// Request is a request to merge a branch and its downstack.
type Request struct {
	Branch string // required

	// NoWait skips waiting for each merge to propagate
	// before proceeding to the next branch.
	// When false (default), each merge is confirmed
	// and the next PR is retargeted to trunk
	// before merging.
	NoWait bool
}

// Handler merges change requests via the forge API.
type Handler struct {
	Log              *silog.Logger    // required
	View             ui.View          // required
	Store            Store            // required
	Service          Service          // required
	RemoteRepository forge.Repository // required
}

// MergeDownstack merges the given branch
// and all its downstack ancestors bottom-up.
func (h *Handler) MergeDownstack(
	ctx context.Context, req *Request,
) error {
	plan, err := h.buildPlan(ctx, req.Branch)
	if err != nil {
		return err
	}

	if len(plan) == 0 {
		h.Log.Info("No open changes to merge.")
		return nil
	}

	if err := h.confirm(plan); err != nil {
		return err
	}

	return h.executePlan(ctx, plan, req.NoWait)
}

// mergeItem is a single branch+change to merge.
type mergeItem struct {
	branch   string
	changeID forge.ChangeID
}

func (h *Handler) buildPlan(
	ctx context.Context, branch string,
) ([]mergeItem, error) {
	downstack, err := h.Service.ListDownstack(ctx, branch)
	if err != nil {
		return nil, fmt.Errorf("list downstack: %w", err)
	}

	// ListDownstack returns top-first; reverse for bottom-up.
	slices.Reverse(downstack)

	items, err := h.resolveChanges(ctx, downstack)
	if err != nil {
		return nil, err
	}

	return h.filterMerged(ctx, items)
}

func (h *Handler) resolveChanges(
	ctx context.Context, branches []string,
) ([]mergeItem, error) {
	var items []mergeItem
	for _, name := range branches {
		resp, err := h.Service.LookupBranch(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("lookup branch %q: %w", name, err)
		}

		if resp.Change == nil {
			return nil, fmt.Errorf(
				"branch %q has no published change request", name,
			)
		}

		items = append(items, mergeItem{
			branch:   name,
			changeID: resp.Change.ChangeID(),
		})
	}
	return items, nil
}

func (h *Handler) filterMerged(
	ctx context.Context, items []mergeItem,
) ([]mergeItem, error) {
	ids := make([]forge.ChangeID, len(items))
	for i, item := range items {
		ids[i] = item.changeID
	}

	states, err := h.RemoteRepository.ChangesStates(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("query change states: %w", err)
	}

	var plan []mergeItem
	for i, item := range items {
		switch states[i] {
		case forge.ChangeMerged:
			h.Log.Infof("%s (%v): already merged, skipping",
				item.branch, item.changeID)
		case forge.ChangeClosed:
			return nil, fmt.Errorf(
				"branch %q (%v) is closed, cannot merge",
				item.branch, item.changeID,
			)
		case forge.ChangeOpen:
			plan = append(plan, item)
		}
	}
	return plan, nil
}

func (h *Handler) confirm(plan []mergeItem) error {
	var desc strings.Builder
	for _, item := range plan {
		fmt.Fprintf(&desc, "  %s (%v)\n", item.branch, item.changeID)
	}

	proceed := true
	prompt := ui.NewConfirm().
		WithTitle(
			fmt.Sprintf(
				"Merge %d change(s) bottom-up?",
				len(plan),
			),
		).
		WithDescription(desc.String()).
		WithValue(&proceed)
	if err := ui.Run(h.View, prompt); err != nil {
		return fmt.Errorf("run prompt: %w", err)
	}

	if !proceed {
		return errors.New("merge aborted")
	}
	return nil
}

func (h *Handler) executePlan(
	ctx context.Context, plan []mergeItem, noWait bool,
) error {
	var trunk string
	if !noWait {
		trunk = h.Store.Trunk()
	}

	for i, item := range plan {
		h.Log.Infof("Merging %s (%v)...",
			item.branch, item.changeID)
		if err := h.RemoteRepository.MergeChange(
			ctx, item.changeID,
		); err != nil {
			return fmt.Errorf("merge %q: %w", item.branch, err)
		}

		lastItem := i == len(plan)-1
		if !noWait && !lastItem {
			if err := h.settleAndRetarget(
				ctx, plan[i+1], item, trunk,
			); err != nil {
				return err
			}
		}
	}

	h.Log.Infof("All %d change(s) merged.", len(plan))
	return nil
}

// settleAndRetarget waits for a merged change to settle,
// then retargets the next change's base to trunk.
func (h *Handler) settleAndRetarget(
	ctx context.Context,
	next, merged mergeItem,
	trunk string,
) error {
	if err := h.awaitMerged(ctx, merged); err != nil {
		return fmt.Errorf("await merge of %q: %w",
			merged.branch, err)
	}

	return h.retargetChange(ctx, next, trunk)
}

// awaitMerged polls until the given change shows as merged.
// Uses exponential backoff starting at 500ms, capped at 8s.
func (h *Handler) awaitMerged(
	ctx context.Context, item mergeItem,
) error {
	const (
		_initialDelay = 500 * time.Millisecond
		_maxDelay     = 8 * time.Second
		_timeout      = 2 * time.Minute
	)

	ctx, cancel := context.WithTimeout(ctx, _timeout)
	defer cancel()

	delay := _initialDelay
	for {
		states, err := h.RemoteRepository.ChangesStates(
			ctx, []forge.ChangeID{item.changeID},
		)
		if err != nil {
			return fmt.Errorf("poll state: %w", err)
		}

		if states[0] == forge.ChangeMerged {
			return nil
		}

		h.Log.Debugf("Waiting for %s to settle...",
			item.branch)
		select {
		case <-ctx.Done():
			return fmt.Errorf(
				"timed out waiting for %q to merge",
				item.branch,
			)
		case <-time.After(delay):
		}

		delay = min(delay*2, _maxDelay)
	}
}

// retargetChange updates the next change's base to trunk.
func (h *Handler) retargetChange(
	ctx context.Context, item mergeItem, trunk string,
) error {
	h.Log.Infof("Retargeting %s to %s...",
		item.branch, trunk)
	err := h.RemoteRepository.EditChange(
		ctx, item.changeID,
		forge.EditChangeOptions{Base: trunk},
	)
	if err != nil {
		return fmt.Errorf("retarget %q to %q: %w",
			item.branch, trunk, err)
	}
	return nil
}
