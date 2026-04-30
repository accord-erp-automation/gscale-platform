package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrintRequestReader_Pending(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "bridge_state.json")
	if err := os.WriteFile(p, []byte(`{"print_request":{"epc":"3034257bf7194e406994036b","qty":2.5,"item_code":"ITM-001","status":"pending"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newPrintRequestReader(p)
	got, ok := r.Pending(time.Now())
	if !ok {
		t.Fatal("expected pending print request")
	}
	if got.EPC != "3034257BF7194E406994036B" {
		t.Fatalf("epc mismatch: %q", got.EPC)
	}
	if got.Qty == nil || *got.Qty != 2.5 {
		t.Fatalf("qty mismatch: %+v", got.Qty)
	}
	if got.ItemName != "ITM-001" {
		t.Fatalf("item name fallback mismatch: %q", got.ItemName)
	}
	if got.Unit != "kg" {
		t.Fatalf("unit fallback mismatch: %q", got.Unit)
	}
	if got.Mode != "rfid" {
		t.Fatalf("mode default mismatch: %q", got.Mode)
	}
}

func TestPrintRequestReader_NotPendingWhenCleared(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "bridge_state.json")
	if err := os.WriteFile(p, []byte(`{"print_request":{"epc":"","status":""}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newPrintRequestReader(p)
	if _, ok := r.Pending(time.Now()); ok {
		t.Fatal("expected no pending print request")
	}
}

func TestPrintRequestReader_NormalizesMode(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "bridge_state.json")
	if err := os.WriteFile(p, []byte(`{"print_request":{"epc":"3034257bf7194e406994036b","qty":2.5,"gross_qty":3.28,"item_code":"ITM-001","status":"pending","mode":"label-only","printer":"g500","tare":true,"tare_kg":0.78}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newPrintRequestReader(p)
	got, ok := r.Pending(time.Now())
	if !ok {
		t.Fatal("expected pending print request")
	}
	if got.Mode != "label" {
		t.Fatalf("mode normalization mismatch: %q", got.Mode)
	}
	if got.Printer != "godex" {
		t.Fatalf("printer normalization mismatch: %q", got.Printer)
	}
	if got.GrossQty == nil || *got.GrossQty != 3.28 {
		t.Fatalf("gross qty mismatch: %+v", got.GrossQty)
	}
	if !got.Tare || got.TareKG != 0.78 {
		t.Fatalf("tare normalization mismatch: enabled=%v kg=%v", got.Tare, got.TareKG)
	}
}
