package main

import (
	"strings"
	"testing"
	"time"
)

func TestBuildRFIDEncodeCommand_IncludesEPCAndQtyOnLabel(t *testing.T) {
	stream, err := buildRFIDEncodeCommand("3034ABCDEF1234567890AABB", "1.250 kg", "GREEN TEA")
	if err != nil {
		t.Fatalf("buildRFIDEncodeCommand error: %v", err)
	}
	if !strings.HasPrefix(stream, "~PS\n^XA\n") {
		t.Fatalf("stream must start with resume+format: %q", stream)
	}

	if !strings.Contains(stream, "^RFW,H,,,A^FD3034ABCDEF1234567890AABB^FS") {
		t.Fatalf("rfid write command not found in stream: %s", stream)
	}
	if !strings.Contains(stream, "^RS8,,,1,N") {
		t.Fatalf("^RS tag type + error-handling flag missing in stream: %s", stream)
	}
	if !strings.Contains(stream, "^FDMAHSULOT: GREEN TEA^FS") {
		t.Fatalf("human MAHSULOT line missing: %s", stream)
	}
	if !strings.Contains(stream, "^FDEPC: 3034ABCDEF1234567890AABB^FS") {
		t.Fatalf("human EPC line missing: %s", stream)
	}
	if !strings.Contains(stream, "^FDVAZNI: 1.250 kg^FS") {
		t.Fatalf("human VAZNI line missing: %s", stream)
	}
	if !strings.Contains(stream, "^BCN,44,N,N,N") {
		t.Fatalf("barcode command missing: %s", stream)
	}
	if strings.Count(stream, "3034ABCDEF1234567890AABB") < 3 {
		t.Fatalf("epc should be present for rfid write, text and barcode: %s", stream)
	}
}

func TestBuildRFIDEncodeCommand_DefaultQtyWhenEmpty(t *testing.T) {
	stream, err := buildRFIDEncodeCommand("3034ABCDEF1234567890AABB", "", "")
	if err != nil {
		t.Fatalf("buildRFIDEncodeCommand error: %v", err)
	}

	if !strings.Contains(stream, "^FDVAZNI: - kg^FS") {
		t.Fatalf("default qty missing: %s", stream)
	}
	if !strings.Contains(stream, "^FDMAHSULOT: -^FS") {
		t.Fatalf("default item missing: %s", stream)
	}
}

func TestBuildLabelOnlyPrintCommand_OmitsRFIDWrite(t *testing.T) {
	stream, err := buildLabelOnlyPrintCommand("3034ABCDEF1234567890AABB", "1.250 kg", "GREEN TEA")
	if err != nil {
		t.Fatalf("buildLabelOnlyPrintCommand error: %v", err)
	}

	if !strings.HasPrefix(stream, "~PS\n^XA\n") {
		t.Fatalf("stream must start with resume+format: %q", stream)
	}
	if strings.Contains(stream, "^RFW,H,,,A") {
		t.Fatalf("label-only stream must not include RFID write: %s", stream)
	}
	if strings.Contains(stream, "^RS8,,,1,N") {
		t.Fatalf("label-only stream must not include RFID setup: %s", stream)
	}
	if !strings.Contains(stream, "^FDMAHSULOT: GREEN TEA^FS") {
		t.Fatalf("human label item missing: %s", stream)
	}
	if !strings.Contains(stream, "^FDEPC: 3034ABCDEF1234567890AABB^FS") {
		t.Fatalf("human EPC line missing: %s", stream)
	}
	if !strings.Contains(stream, "^BCN,44,N,N,N") {
		t.Fatalf("barcode command missing: %s", stream)
	}
}

func TestBuildLabelOnlyPrintCommand_WithTareShowsNettoAndBrutto(t *testing.T) {
	stream, err := buildLabelOnlyPrintCommandWithWeights("3034ABCDEF1234567890AABB", "4.22 kg", "5 kg", "GREEN TEA")
	if err != nil {
		t.Fatalf("buildLabelOnlyPrintCommandWithWeights error: %v", err)
	}

	if !strings.Contains(stream, "^FDNETTO: 4.22 kg^FS") {
		t.Fatalf("netto line missing: %s", stream)
	}
	if !strings.Contains(stream, "^FDBRUTTO: 5 kg^FS") {
		t.Fatalf("brutto line missing: %s", stream)
	}
	if strings.Contains(stream, "^FDVAZNI:") {
		t.Fatalf("tare label should not use generic VAZNI line: %s", stream)
	}
}

func TestBuildLabelOnlyPrintCommand_WithoutTareShowsSameNettoAndBrutto(t *testing.T) {
	stream, err := buildLabelOnlyPrintCommandWithWeights("3034ABCDEF1234567890AABB", "3.2 kg", "3.2 kg", "GREEN TEA")
	if err != nil {
		t.Fatalf("buildLabelOnlyPrintCommandWithWeights error: %v", err)
	}

	if !strings.Contains(stream, "^FDNETTO: 3.2 kg^FS") {
		t.Fatalf("netto line missing: %s", stream)
	}
	if !strings.Contains(stream, "^FDBRUTTO: 3.2 kg^FS") {
		t.Fatalf("brutto line missing: %s", stream)
	}
	if strings.Contains(stream, "^FDVAZNI:") {
		t.Fatalf("no-tare label should not use generic VAZNI line: %s", stream)
	}
}

func TestNormalizeEPC_RejectsNonWordAligned(t *testing.T) {
	// 22 belgi: 22%4=2 — rad etilishi kerak
	_, err := normalizeEPC("3034ABCDEF1234567890AA")
	if err == nil {
		t.Fatal("22-char EPC should be rejected (not 16-bit word aligned)")
	}
}

func TestNormalizeEPC_Accepts24Char(t *testing.T) {
	v, err := normalizeEPC("3034abcdef1234567890aabb")
	if err != nil {
		t.Fatalf("24-char EPC should be accepted: %v", err)
	}
	if v != "3034ABCDEF1234567890AABB" {
		t.Fatalf("expected uppercase, got %q", v)
	}
}

func TestFormatLabelQty(t *testing.T) {
	w := 1.892
	got := formatLabelQty(&w, "kg")
	if got != "1.9 kg" {
		t.Fatalf("formatLabelQty mismatch: got=%q", got)
	}

	got = formatLabelQty(nil, "kg")
	if got != "- kg" {
		t.Fatalf("formatLabelQty nil mismatch: got=%q", got)
	}
}

func TestGenerateTestEPC_LengthAndUniq(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 123_456_789)
	a := generateTestEPC(t0)
	b := generateTestEPC(t0)

	if len(a) != 24 || len(b) != 24 {
		t.Fatalf("epc len mismatch: a=%d b=%d", len(a), len(b))
	}
	if a == b {
		t.Fatalf("expected unique epc for same tick: %s", a)
	}
	if strings.HasSuffix(a, "00000000") || strings.HasSuffix(b, "00000000") {
		t.Fatalf("epc tail should not be all-zero: a=%s b=%s", a, b)
	}
	if !isUpperHexScale(a) || !isUpperHexScale(b) {
		t.Fatalf("epc must be uppercase hex: a=%s b=%s", a, b)
	}
}

func isUpperHexScale(v string) bool {
	for _, ch := range v {
		if strings.ContainsRune("0123456789ABCDEF", ch) {
			continue
		}
		return false
	}
	return true
}
