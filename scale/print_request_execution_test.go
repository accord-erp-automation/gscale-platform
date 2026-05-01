package main

import (
	bridgestate "bridge/state"
	"path/filepath"
	"testing"
)

func TestDecidePendingPrintRequest(t *testing.T) {
	req := bridgestate.PrintRequestSnapshot{
		EPC:    "3034257BF7194E406994036B",
		Status: "pending",
	}

	if got := decidePendingPrintRequest(req, ZebraStatus{}, "", false, Reading{}); got != printRequestErrorDisabled {
		t.Fatalf("disabled decision mismatch: got=%s", got)
	}

	if got := decidePendingPrintRequest(req, ZebraStatus{}, "", false, Reading{Port: "polygon://scale"}); got != printRequestExternalExec {
		t.Fatalf("polygon bridge decision mismatch: got=%s", got)
	}

	if got := decidePendingPrintRequest(req, ZebraStatus{}, "", true, Reading{}); got != printRequestExecute {
		t.Fatalf("execute decision mismatch: got=%s", got)
	}

	if got := decidePendingPrintRequest(req, ZebraStatus{}, "3034257BF7194E406994036B", true, Reading{}); got != printRequestNoop {
		t.Fatalf("active request should noop: got=%s", got)
	}

	zebra := ZebraStatus{LastEPC: "3034257BF7194E406994036B"}
	if got := decidePendingPrintRequest(req, zebra, "", true, Reading{}); got != printRequestMarkDone {
		t.Fatalf("matching epc should mark done: got=%s", got)
	}

	req.Mode = "label"
	if got := decidePendingPrintRequest(req, zebra, "", true, Reading{}); got != printRequestExecute {
		t.Fatalf("label-only mode should still execute: got=%s", got)
	}
}

func TestWritePrintRequestStatus_UpdatesMatchingEPCOnly(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "bridge_state.json")
	s := bridgestate.New(p)

	qty := 2.5
	if err := s.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.PrintRequest.EPC = "3034257BF7194E406994036B"
		snapshot.PrintRequest.Qty = &qty
		snapshot.PrintRequest.Status = "pending"
	}); err != nil {
		t.Fatalf("seed update error: %v", err)
	}

	if err := writePrintRequestStatus(s, "AAAAAAAAAAAAAAAAAAAAAAAA", "done", ""); err != nil {
		t.Fatalf("write status mismatch epc error: %v", err)
	}
	got, err := s.Read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if got.PrintRequest.Status != "pending" {
		t.Fatalf("status should stay pending for mismatched epc: %q", got.PrintRequest.Status)
	}

	if err := writePrintRequestStatus(s, "3034257BF7194E406994036B", "done", ""); err != nil {
		t.Fatalf("write status error: %v", err)
	}
	got, err = s.Read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if got.PrintRequest.Status != "done" {
		t.Fatalf("status mismatch: %q", got.PrintRequest.Status)
	}
}

func TestFormatPrintWeightLabels_WithTare(t *testing.T) {
	net := 1.892
	gross := 5.0
	req := bridgestate.PrintRequestSnapshot{
		Qty:      &net,
		GrossQty: &gross,
		Unit:     "kg",
		Tare:     true,
		TareKG:   0.78,
	}

	got := formatPrintWeightLabels(req)
	if !got.HasTare {
		t.Fatal("expected tare labels")
	}
	if got.Netto != "1.9 kg" {
		t.Fatalf("netto mismatch: %q", got.Netto)
	}
	if got.Brutto != "5 kg" {
		t.Fatalf("brutto mismatch: %q", got.Brutto)
	}

	godex := formatGoDEXWeightLabels(req, "5kg")
	if godex.Netto != "1.9" || godex.Brutto != "5" {
		t.Fatalf("godex labels mismatch: %+v", godex)
	}
}
