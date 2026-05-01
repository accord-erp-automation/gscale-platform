package mobileapi

import (
	bridgestate "bridge/state"
	"context"
	"testing"
	"time"
)

func TestWaitForNextCycleRequiresScaleRelease(t *testing.T) {
	stateFile := t.TempDir() + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	client := workflowBridgeClient{store: store}

	weight := 3.2
	stable := true
	if err := store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.Scale = bridgestate.ScaleSnapshot{
			Weight:    &weight,
			Unit:      "kg",
			Stable:    &stable,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
	}); err != nil {
		t.Fatalf("seed scale state: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- client.WaitForNextCycle(ctx, 1500*time.Millisecond, 50*time.Millisecond, 3.2)
	}()

	time.Sleep(150 * time.Millisecond)
	if err := store.Update(func(snapshot *bridgestate.Snapshot) {
		zero := 0.0
		snapshot.Scale = bridgestate.ScaleSnapshot{
			Weight:    &zero,
			Unit:      "kg",
			Stable:    &stable,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
	}); err != nil {
		t.Fatalf("clear scale state: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wait next cycle error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("wait next cycle did not release after zero")
	}
}
