package erp

import (
	"context"
	"os"
	"testing"
)

func TestReadServiceIntegration(t *testing.T) {
	readURL := os.Getenv("ERP_READ_URL")
	if readURL == "" {
		t.Skip("ERP_READ_URL not set")
	}

	c := NewWithReadURL("https://erp.invalid", "k", "s", readURL)

	user, err := c.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection error: %v", err)
	}
	if user == "" {
		t.Fatalf("empty read service identity")
	}

	items, err := c.SearchItems(context.Background(), "zaxro", 3)
	if err != nil {
		t.Fatalf("SearchItems error: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected non-empty items")
	}

	if _, err := c.SearchItemWarehouses(context.Background(), items[0].ItemCode, "", 3); err != nil {
		t.Fatalf("SearchItemWarehouses error: %v", err)
	}

	uom, err := c.lookupItemStockUOM(context.Background(), items[0].ItemCode)
	if err != nil {
		t.Fatalf("lookupItemStockUOM error: %v", err)
	}
	if uom == "" {
		t.Fatalf("empty stock uom")
	}

	company, err := c.lookupWarehouseCompany(context.Background(), "Stores - A")
	if err != nil {
		t.Fatalf("lookupWarehouseCompany error: %v", err)
	}
	if company == "" {
		t.Fatalf("empty warehouse company")
	}
}
