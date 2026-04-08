package main

import (
	bridgestate "bridge/state"
	"strings"
	"testing"
	"time"
)

func TestBootstrapWritesInitialSnapshot(t *testing.T) {
	t.Parallel()

	stateFile := t.TempDir() + "/bridge_state.json"
	sim := newSimulator(config{
		bridgeStateFile: stateFile,
		auto:            true,
		printMode:       "success",
		printDelay:      time.Second,
	})

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	if err := sim.bootstrap(now); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	snap, err := bridgestate.New(stateFile).Read()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if snap.Scale.Source != "polygon" {
		t.Fatalf("scale source = %q", snap.Scale.Source)
	}
	if snap.Scale.Weight == nil {
		t.Fatalf("scale weight is nil")
	}
	if snap.Zebra.DevicePath != "polygon://zebra" {
		t.Fatalf("zebra device = %q", snap.Zebra.DevicePath)
	}
}

func TestTickCompletesPendingPrintRequest(t *testing.T) {
	t.Parallel()

	stateFile := t.TempDir() + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)

	sim := newSimulator(config{
		bridgeStateFile: stateFile,
		auto:            false,
		printMode:       "success",
		printDelay:      time.Second,
	})
	if err := sim.bootstrap(now); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	qty := 1.25
	if err := store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.PrintRequest = bridgestate.PrintRequestSnapshot{
			EPC:         "3034257BF7194E406994036B",
			Qty:         &qty,
			Unit:        "kg",
			ItemCode:    "ITEM-001",
			ItemName:    "Polygon Test",
			Status:      "pending",
			RequestedAt: now.UTC().Format(time.RFC3339Nano),
			UpdatedAt:   now.UTC().Format(time.RFC3339Nano),
		}
	}); err != nil {
		t.Fatalf("seed print request: %v", err)
	}

	if err := sim.tick(now.Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("tick start: %v", err)
	}
	if err := sim.tick(now.Add(1200 * time.Millisecond)); err != nil {
		t.Fatalf("tick finish: %v", err)
	}

	snap, err := store.Read()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if snap.PrintRequest.Status != "done" {
		t.Fatalf("print request status = %q", snap.PrintRequest.Status)
	}
	if snap.Zebra.LastEPC != "3034257BF7194E406994036B" {
		t.Fatalf("zebra last_epc = %q", snap.Zebra.LastEPC)
	}
	if snap.Zebra.Verify != "WRITTEN" {
		t.Fatalf("zebra verify = %q", snap.Zebra.Verify)
	}
	if len(sim.printerHistory) != 1 {
		t.Fatalf("printer history len = %d", len(sim.printerHistory))
	}
	if sim.printerHistory[0].Status != "done" {
		t.Fatalf("printer history status = %q", sim.printerHistory[0].Status)
	}
	if !strings.Contains(sim.printerHistory[0].Preview, "^RFW,H,,,A^FD3034257BF7194E406994036B^FS") {
		t.Fatalf("printer preview missing EPC write command: %s", sim.printerHistory[0].Preview)
	}
}
