package batchcontrol

import (
	"context"
	"errors"
	"testing"
	"time"

	"core/workflow"
)

func TestOtherActiveBatchOwner(t *testing.T) {
	t.Parallel()

	service := &Service{
		batchByID: map[int64]batchSession{
			1001: {id: 1},
		},
	}

	if owner, ok := service.OtherActiveBatchOwner(1001); ok || owner != 0 {
		t.Fatalf("same owner should not be treated as other active owner: owner=%d ok=%v", owner, ok)
	}

	owner, ok := service.OtherActiveBatchOwner(2002)
	if !ok {
		t.Fatal("expected another active owner")
	}
	if owner != 1001 {
		t.Fatalf("owner mismatch: got=%d want=%d", owner, 1001)
	}
}

func TestStartAndStopSyncBatchState(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := &stubBatchStateWriter{}
	runner := &stubRunner{
		runStarted: make(chan struct{}, 1),
		runDone:    make(chan struct{}, 1),
	}
	service := New(Dependencies{
		Runner:     runner,
		BatchState: state,
	})

	started := service.Start(ctx, 1001, workflow.Selection{
		ItemCode:  "ITEM-1",
		ItemName:  "Tea",
		Warehouse: "Stores - A",
	}, workflow.Hooks{})
	if !started {
		t.Fatal("expected batch to start")
	}

	select {
	case <-runner.runStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	if len(state.calls) == 0 || !state.calls[0].active {
		t.Fatalf("expected active batch state call, got %#v", state.calls)
	}

	if !service.Stop(1001) {
		t.Fatal("expected stop to cancel active batch")
	}

	select {
	case <-runner.runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not stop")
	}

	if got := state.calls[len(state.calls)-1]; got.active {
		t.Fatalf("expected final state inactive, got %#v", got)
	}
}

func TestStartRejectsOtherOwner(t *testing.T) {
	t.Parallel()

	service := New(Dependencies{
		Runner: &stubRunner{},
	})
	service.batchByID[1001] = batchSession{id: 1}

	started := service.Start(context.Background(), 2002, workflow.Selection{
		ItemCode:  "ITEM-1",
		Warehouse: "Stores - A",
	}, workflow.Hooks{})
	if started {
		t.Fatal("expected start to fail when another owner is active")
	}
}

type stubRunner struct {
	runStarted chan struct{}
	runDone    chan struct{}
}

func (s *stubRunner) Run(ctx context.Context, selection workflow.Selection, hooks workflow.Hooks) error {
	if s.runStarted != nil {
		s.runStarted <- struct{}{}
	}
	<-ctx.Done()
	if s.runDone != nil {
		s.runDone <- struct{}{}
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil
	}
	return ctx.Err()
}

type batchStateCall struct {
	active    bool
	ownerID   int64
	selection workflow.Selection
}

type stubBatchStateWriter struct {
	calls []batchStateCall
}

func (s *stubBatchStateWriter) Set(active bool, ownerID int64, selection workflow.Selection) error {
	s.calls = append(s.calls, batchStateCall{
		active:    active,
		ownerID:   ownerID,
		selection: selection,
	})
	return nil
}
