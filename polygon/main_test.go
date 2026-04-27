package main

import (
	bridgestate "bridge/state"
	"net/http"
	"net/http/httptest"
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
		scenario:        "batch-flow",
		seed:            42,
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

func TestIdleScenarioStaysNearZero(t *testing.T) {
	t.Parallel()

	cycle := buildScenarioCycle("idle", 42)
	if len(cycle) == 0 {
		t.Fatal("idle cycle is empty")
	}
	for _, frame := range cycle {
		if !frame.stable {
			t.Fatal("idle scenario should stay stable")
		}
		if frame.weight > 0.010 {
			t.Fatalf("idle weight too high: %.3f", frame.weight)
		}
	}
}

func TestScenarioEndpointSwitchesProfiles(t *testing.T) {
	t.Parallel()

	sim := newSimulator(config{
		bridgeStateFile: t.TempDir() + "/bridge_state.json",
		auto:            true,
		printMode:       "success",
		scenario:        "batch-flow",
		seed:            42,
		printDelay:      time.Second,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/dev/scenario", strings.NewReader(`{"scenario":"stress","seed":7,"auto":false}`))
	rec := httptest.NewRecorder()
	sim.handleScenario(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if sim.scenario != "stress" {
		t.Fatalf("scenario = %q", sim.scenario)
	}
	if sim.seed != 7 {
		t.Fatalf("seed = %d", sim.seed)
	}
	if len(sim.cycle) == 0 {
		t.Fatal("stress cycle is empty")
	}
	if sim.auto {
		t.Fatal("scenario switch should preserve auto=false from payload")
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
	if snap.Zebra.DeviceState != "ready" {
		t.Fatalf("zebra device state = %q", snap.Zebra.DeviceState)
	}
	if snap.Zebra.MediaState != "ok" {
		t.Fatalf("zebra media state = %q", snap.Zebra.MediaState)
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

func TestPrinterSimulationCanBeDisabled(t *testing.T) {
	t.Parallel()

	stateFile := t.TempDir() + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	qty := 1.25

	if err := store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.Zebra = bridgestate.ZebraSnapshot{
			Connected:   true,
			DevicePath:  "/dev/usb/lp0",
			Name:        "Real Zebra",
			DeviceState: "ready",
			Verify:      "REAL",
		}
		snapshot.PrintRequest = bridgestate.PrintRequestSnapshot{
			EPC:         "3034257BF7194E406994036B",
			Qty:         &qty,
			Unit:        "kg",
			ItemCode:    "ITEM-001",
			ItemName:    "Real Printer Test",
			Status:      "pending",
			RequestedAt: now.UTC().Format(time.RFC3339Nano),
			UpdatedAt:   now.UTC().Format(time.RFC3339Nano),
		}
	}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	sim := newSimulator(config{
		bridgeStateFile: stateFile,
		disablePrinter:  true,
		auto:            false,
		printMode:       "success",
		printDelay:      time.Second,
	})
	if err := sim.bootstrap(now); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := sim.tick(now.Add(1200 * time.Millisecond)); err != nil {
		t.Fatalf("tick: %v", err)
	}

	snap, err := store.Read()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if snap.PrintRequest.Status != "pending" {
		t.Fatalf("print request status = %q", snap.PrintRequest.Status)
	}
	if snap.Zebra.DevicePath != "/dev/usb/lp0" {
		t.Fatalf("zebra device was overwritten: %q", snap.Zebra.DevicePath)
	}
	if len(sim.printerHistory) != 0 {
		t.Fatalf("printer history len = %d", len(sim.printerHistory))
	}
	if snap.Scale.Source != "polygon" {
		t.Fatalf("scale source = %q", snap.Scale.Source)
	}
}

func TestDefaultCycleHasBurstAndPausePattern(t *testing.T) {
	t.Parallel()

	sim := newSimulator(config{
		bridgeStateFile: t.TempDir() + "/bridge_state.json",
		auto:            true,
		printMode:       "success",
		printDelay:      time.Second,
	})

	if len(sim.cycle) < 12 {
		t.Fatalf("cycle len = %d, want richer burst pattern", len(sim.cycle))
	}

	hasFastBurst := false
	hasStablePause := false
	hasZeroPause := false
	for _, frame := range sim.cycle {
		if !frame.stable && frame.weight > 0 && frame.duration <= 250*time.Millisecond {
			hasFastBurst = true
		}
		if frame.stable && frame.weight > 0 && frame.duration >= 4500*time.Millisecond {
			hasStablePause = true
		}
		if frame.weight == 0 && frame.duration >= 900*time.Millisecond {
			hasZeroPause = true
		}
	}

	if !hasFastBurst {
		t.Fatal("cycle should have fast moving burst frames")
	}
	if !hasStablePause {
		t.Fatal("cycle should have stable pause frames")
	}
	if !hasZeroPause {
		t.Fatal("cycle should have zero-weight pause frames")
	}
}

func TestBatchFlowStableFramesDoNotProduceTinyPositiveWeights(t *testing.T) {
	t.Parallel()

	cycle := buildScenarioCycle("batch-flow", 42)
	if len(cycle) == 0 {
		t.Fatal("batch-flow cycle is empty")
	}
	for _, frame := range cycle {
		if frame.stable && frame.weight > 0 && frame.weight < 0.05 {
			t.Fatalf("stable frame should not have tiny positive weight: %.3f", frame.weight)
		}
	}
}
