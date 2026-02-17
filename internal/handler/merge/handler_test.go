package merge

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"go.abhg.dev/gs/internal/forge"
	"go.abhg.dev/gs/internal/forge/forgetest"
	"go.abhg.dev/gs/internal/silog"
	"go.abhg.dev/gs/internal/ui"
)

//go:generate mockgen -destination=mocks_test.go -package=merge -write_package_comment=false -typed=true . Service,Store

// fakeChangeID is a simple string-based ChangeID for testing.
type fakeChangeID string

func (f fakeChangeID) String() string { return string(f) }

func TestAwaitMerged_immediate(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := forgetest.NewMockRepository(ctrl)
	mockRepo.EXPECT().
		ChangesStates(
			gomock.Any(),
			[]forge.ChangeID{fakeChangeID("pr-1")},
		).
		Return([]forge.ChangeState{forge.ChangeMerged}, nil)

	h := &Handler{
		Log:              silog.Nop(),
		View:             &ui.FileView{W: io.Discard},
		Store:            NewMockStore(ctrl),
		Service:          NewMockService(ctrl),
		RemoteRepository: mockRepo,
	}

	err := h.awaitMerged(t.Context(), mergeItem{
		branch:   "feat1",
		changeID: fakeChangeID("pr-1"),
	})
	require.NoError(t, err)
}

func TestAwaitMerged_afterPolling(t *testing.T) {
	ctrl := gomock.NewController(t)

	ids := []forge.ChangeID{fakeChangeID("pr-1")}
	mockRepo := forgetest.NewMockRepository(ctrl)

	// First call: still open.
	mockRepo.EXPECT().
		ChangesStates(gomock.Any(), ids).
		Return([]forge.ChangeState{forge.ChangeOpen}, nil)
	// Second call: merged.
	mockRepo.EXPECT().
		ChangesStates(gomock.Any(), ids).
		Return([]forge.ChangeState{forge.ChangeMerged}, nil)

	h := &Handler{
		Log:              silog.Nop(),
		View:             &ui.FileView{W: io.Discard},
		Store:            NewMockStore(ctrl),
		Service:          NewMockService(ctrl),
		RemoteRepository: mockRepo,
	}

	err := h.awaitMerged(t.Context(), mergeItem{
		branch:   "feat1",
		changeID: fakeChangeID("pr-1"),
	})
	require.NoError(t, err)
}

func TestExecutePlan_retargets(t *testing.T) {
	ctrl := gomock.NewController(t)
	var logBuffer bytes.Buffer

	mockRepo := forgetest.NewMockRepository(ctrl)
	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().Trunk().Return("main").AnyTimes()

	pr1 := fakeChangeID("pr-1")
	pr2 := fakeChangeID("pr-2")
	pr3 := fakeChangeID("pr-3")

	// Merge pr-1, await, retarget pr-2.
	mockRepo.EXPECT().
		MergeChange(gomock.Any(), pr1).
		Return(nil)
	mockRepo.EXPECT().
		ChangesStates(gomock.Any(),
			[]forge.ChangeID{pr1}).
		Return([]forge.ChangeState{forge.ChangeMerged}, nil)
	mockRepo.EXPECT().
		EditChange(gomock.Any(), pr2,
			forge.EditChangeOptions{Base: "main"}).
		Return(nil)

	// Merge pr-2, await, retarget pr-3.
	mockRepo.EXPECT().
		MergeChange(gomock.Any(), pr2).
		Return(nil)
	mockRepo.EXPECT().
		ChangesStates(gomock.Any(),
			[]forge.ChangeID{pr2}).
		Return([]forge.ChangeState{forge.ChangeMerged}, nil)
	mockRepo.EXPECT().
		EditChange(gomock.Any(), pr3,
			forge.EditChangeOptions{Base: "main"}).
		Return(nil)

	// Merge pr-3 (last, no await/retarget).
	mockRepo.EXPECT().
		MergeChange(gomock.Any(), pr3).
		Return(nil)

	h := &Handler{
		Log:              silog.New(&logBuffer, nil),
		View:             &ui.FileView{W: io.Discard},
		Store:            mockStore,
		Service:          NewMockService(ctrl),
		RemoteRepository: mockRepo,
	}

	plan := []mergeItem{
		{branch: "feat1", changeID: pr1},
		{branch: "feat2", changeID: pr2},
		{branch: "feat3", changeID: pr3},
	}

	err := h.executePlan(t.Context(), plan, false)
	require.NoError(t, err)

	output := logBuffer.String()
	assert.Contains(t, output, "Merging feat1")
	assert.Contains(t, output, "Retargeting feat2 to main")
	assert.Contains(t, output, "Merging feat2")
	assert.Contains(t, output, "Retargeting feat3 to main")
	assert.Contains(t, output, "Merging feat3")
	assert.Contains(t, output, "All 3 change(s) merged")
}

func TestExecutePlan_noWait(t *testing.T) {
	ctrl := gomock.NewController(t)
	var logBuffer bytes.Buffer

	mockRepo := forgetest.NewMockRepository(ctrl)

	pr1 := fakeChangeID("pr-1")
	pr2 := fakeChangeID("pr-2")

	// Only MergeChange calls, no ChangesStates or EditChange.
	mockRepo.EXPECT().
		MergeChange(gomock.Any(), pr1).
		Return(nil)
	mockRepo.EXPECT().
		MergeChange(gomock.Any(), pr2).
		Return(nil)

	h := &Handler{
		Log:              silog.New(&logBuffer, nil),
		View:             &ui.FileView{W: io.Discard},
		Store:            NewMockStore(ctrl),
		Service:          NewMockService(ctrl),
		RemoteRepository: mockRepo,
	}

	plan := []mergeItem{
		{branch: "feat1", changeID: pr1},
		{branch: "feat2", changeID: pr2},
	}

	err := h.executePlan(t.Context(), plan, true)
	require.NoError(t, err)

	output := logBuffer.String()
	assert.Contains(t, output, "Merging feat1")
	assert.Contains(t, output, "Merging feat2")
	assert.NotContains(t, output, "Retargeting")
}

func TestExecutePlan_singleBranch(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := forgetest.NewMockRepository(ctrl)
	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().Trunk().Return("main")

	pr1 := fakeChangeID("pr-1")

	// Single merge, no await/retarget.
	mockRepo.EXPECT().
		MergeChange(gomock.Any(), pr1).
		Return(nil)

	h := &Handler{
		Log:              silog.Nop(),
		View:             &ui.FileView{W: io.Discard},
		Store:            mockStore,
		Service:          NewMockService(ctrl),
		RemoteRepository: mockRepo,
	}

	plan := []mergeItem{
		{branch: "feat1", changeID: pr1},
	}

	err := h.executePlan(t.Context(), plan, false)
	require.NoError(t, err)
}
