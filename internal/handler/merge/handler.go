// Package merge implements the branch merge command.
package merge

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

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

	return h.executePlan(ctx, plan)
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
	ctx context.Context, plan []mergeItem,
) error {
	for _, item := range plan {
		h.Log.Infof("Merging %s (%v)...", item.branch, item.changeID)
		if err := h.RemoteRepository.MergeChange(
			ctx, item.changeID,
		); err != nil {
			return fmt.Errorf("merge %q: %w", item.branch, err)
		}
	}

	h.Log.Infof("All %d change(s) merged.", len(plan))
	return nil
}
