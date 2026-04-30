package mobileapi

import (
	"context"
	"core/workflow"
	"strings"
	"testing"
)

func TestDevERPWriteClientCreatesAndSubmitsDraft(t *testing.T) {
	t.Parallel()

	client := newDevERPWriteClient()
	draft, err := client.CreateMaterialReceiptDraft(context.Background(), workflow.CreateMaterialReceiptDraftInput{
		ItemCode:  " zizi ",
		Warehouse: " Stores - A ",
		Qty:       4.22,
		Barcode:   " abc123 ",
	})
	if err != nil {
		t.Fatalf("CreateMaterialReceiptDraft: %v", err)
	}
	if !strings.HasPrefix(draft.Name, "DEV-MR-") {
		t.Fatalf("draft name = %q", draft.Name)
	}
	if draft.ItemCode != "zizi" || draft.Warehouse != "Stores - A" || draft.Barcode != "ABC123" {
		t.Fatalf("draft normalized fields = %#v", draft)
	}
	if draft.Qty != 4.22 || draft.UOM != "Kg" {
		t.Fatalf("draft qty/uom = %#v", draft)
	}
	if err := client.SubmitStockEntryDraft(context.Background(), draft.Name); err != nil {
		t.Fatalf("SubmitStockEntryDraft: %v", err)
	}
	if err := client.DeleteStockEntryDraft(context.Background(), draft.Name); err != nil {
		t.Fatalf("DeleteStockEntryDraft: %v", err)
	}
}

func TestConfigCanRunBatchActionsWithDevERPWrite(t *testing.T) {
	t.Parallel()

	cfg := Config{DevERPWrite: true}
	if cfg.HasERPWriteConfig() {
		t.Fatal("HasERPWriteConfig should stay false without real ERP credentials")
	}
	if !cfg.CanRunBatchActions() {
		t.Fatal("CanRunBatchActions should be true with dev ERP write enabled")
	}
}
