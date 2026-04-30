package mobileapi

import (
	"context"
	"core/workflow"
	"fmt"
	"strings"
	"sync/atomic"
)

type devERPWriteClient struct {
	seq atomic.Uint64
}

func newDevERPWriteClient() *devERPWriteClient {
	return &devERPWriteClient{}
}

func (c *devERPWriteClient) CreateMaterialReceiptDraft(_ context.Context, in workflow.CreateMaterialReceiptDraftInput) (workflow.Draft, error) {
	in.ItemCode = strings.TrimSpace(in.ItemCode)
	in.Warehouse = strings.TrimSpace(in.Warehouse)
	in.Barcode = strings.ToUpper(strings.TrimSpace(in.Barcode))
	if in.ItemCode == "" {
		return workflow.Draft{}, fmt.Errorf("item code bo'sh")
	}
	if in.Warehouse == "" {
		return workflow.Draft{}, fmt.Errorf("warehouse bo'sh")
	}
	if in.Qty <= 0 {
		return workflow.Draft{}, fmt.Errorf("qty > 0 bo'lishi kerak")
	}
	return workflow.Draft{
		Name:      fmt.Sprintf("DEV-MR-%06d", c.seq.Add(1)),
		ItemCode:  in.ItemCode,
		Warehouse: in.Warehouse,
		Qty:       in.Qty,
		UOM:       "Kg",
		Barcode:   in.Barcode,
	}, nil
}

func (c *devERPWriteClient) SubmitStockEntryDraft(_ context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("stock entry name bo'sh")
	}
	return nil
}

func (c *devERPWriteClient) DeleteStockEntryDraft(_ context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("stock entry name bo'sh")
	}
	return nil
}
