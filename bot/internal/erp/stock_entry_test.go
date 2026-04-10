package erp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateMaterialReceiptDraft(t *testing.T) {
	t.Helper()

	type posted struct {
		StockEntryType string `json:"stock_entry_type"`
		Company        string `json:"company"`
		ToWarehouse    string `json:"to_warehouse"`
		Items          []struct {
			ItemCode   string  `json:"item_code"`
			Warehouse  string  `json:"t_warehouse"`
			Qty        float64 `json:"qty"`
			UOM        string  `json:"uom"`
			StockUOM   string  `json:"stock_uom"`
			Conversion float64 `json:"conversion_factor"`
			Barcode    string  `json:"barcode"`
		} `json:"items"`
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token k:s" {
			t.Fatalf("auth header mismatch: %q", got)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/resource/Warehouse":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"name":"Stores - A","company":"Accord"}]}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/resource/Item":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"name":"ITEM-1","stock_uom":"Kg"}]}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/resource/Stock Entry":
			var p posted
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				t.Fatalf("decode post payload: %v", err)
			}
			if p.StockEntryType != "Material Receipt" {
				t.Fatalf("stock_entry_type mismatch: %q", p.StockEntryType)
			}
			if p.Company != "Accord" || p.ToWarehouse != "Stores - A" {
				t.Fatalf("header fields mismatch: %+v", p)
			}
			if len(p.Items) != 1 {
				t.Fatalf("items len mismatch: %d", len(p.Items))
			}
			if p.Items[0].ItemCode != "ITEM-1" || p.Items[0].Warehouse != "Stores - A" || p.Items[0].Qty != 2.5 {
				t.Fatalf("item payload mismatch: %+v", p.Items[0])
			}
			if p.Items[0].Barcode != "3034257BF7194E406994036B" {
				t.Fatalf("item barcode mismatch: %+v", p.Items[0])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"name":"MAT-STE-2026-00001"}}`))
			return
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	c := New(ts.URL, "k", "s")
	draft, err := c.CreateMaterialReceiptDraft(context.Background(), MaterialReceiptDraftInput{
		ItemCode:  "ITEM-1",
		Warehouse: "Stores - A",
		Qty:       2.5,
		Barcode:   "3034257BF7194E406994036B",
	})
	if err != nil {
		t.Fatalf("CreateMaterialReceiptDraft error: %v", err)
	}
	if draft.Name != "MAT-STE-2026-00001" {
		t.Fatalf("draft name mismatch: %q", draft.Name)
	}
	if draft.UOM != "Kg" {
		t.Fatalf("draft uom mismatch: %q", draft.UOM)
	}
	if draft.Barcode != "3034257BF7194E406994036B" {
		t.Fatalf("draft barcode mismatch: %q", draft.Barcode)
	}
}

func TestCreateMaterialReceiptDraft_UsesReadServiceForLookups(t *testing.T) {
	t.Helper()

	type posted struct {
		Company     string `json:"company"`
		ToWarehouse string `json:"to_warehouse"`
		Items       []struct {
			ItemCode string `json:"item_code"`
			UOM      string `json:"uom"`
		} `json:"items"`
	}

	readTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("unexpected read auth header: %q", got)
		}
		switch r.URL.Path {
		case "/v1/warehouses/Stores - A":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"name":"Stores - A","company":"Accord"}}`))
		case "/v1/items/ITEM-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"name":"ITEM-1","item_code":"ITEM-1","item_name":"Item 1","stock_uom":"Kg"}}`))
		default:
			t.Fatalf("unexpected read request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer readTS.Close()

	writeTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token k:s" {
			t.Fatalf("auth header mismatch: %q", got)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/resource/Stock Entry" {
			t.Fatalf("unexpected write request: %s %s", r.Method, r.URL.Path)
		}

		var p posted
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Fatalf("decode post payload: %v", err)
		}
		if p.Company != "Accord" || p.ToWarehouse != "Stores - A" {
			t.Fatalf("header fields mismatch: %+v", p)
		}
		if len(p.Items) != 1 || p.Items[0].ItemCode != "ITEM-1" || p.Items[0].UOM != "Kg" {
			t.Fatalf("items mismatch: %+v", p.Items)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"name":"MAT-STE-2026-00002"}}`))
	}))
	defer writeTS.Close()

	c := NewWithReadURL(writeTS.URL, "k", "s", readTS.URL)
	draft, err := c.CreateMaterialReceiptDraft(context.Background(), MaterialReceiptDraftInput{
		ItemCode:  "ITEM-1",
		Warehouse: "Stores - A",
		Qty:       2.5,
	})
	if err != nil {
		t.Fatalf("CreateMaterialReceiptDraft error: %v", err)
	}
	if draft.Name != "MAT-STE-2026-00002" {
		t.Fatalf("draft name mismatch: %q", draft.Name)
	}
}

func TestCreateMaterialReceiptDraft_Validate(t *testing.T) {
	c := New("https://example.invalid", "k", "s")
	_, err := c.CreateMaterialReceiptDraft(context.Background(), MaterialReceiptDraftInput{ItemCode: "", Warehouse: "W", Qty: 1})
	if err == nil || !strings.Contains(err.Error(), "item code") {
		t.Fatalf("expected item code error, got: %v", err)
	}
}

func TestSubmitStockEntryDraft(t *testing.T) {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token k:s" {
			t.Fatalf("auth header mismatch: %q", got)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/resource/Stock Entry/MAT-STE-2026-00001":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"doctype":"Stock Entry","name":"MAT-STE-2026-00001","docstatus":0}}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/method/frappe.client.submit":
			var payload struct {
				Doc map[string]any `json:"doc"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode submit payload: %v", err)
			}
			if strings.TrimSpace(fmt.Sprint(payload.Doc["name"])) != "MAT-STE-2026-00001" {
				t.Fatalf("submit doc mismatch: %+v", payload.Doc)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":{"name":"MAT-STE-2026-00001","docstatus":1}}`))
			return
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	c := New(ts.URL, "k", "s")
	if err := c.SubmitStockEntryDraft(context.Background(), "MAT-STE-2026-00001"); err != nil {
		t.Fatalf("SubmitStockEntryDraft error: %v", err)
	}
}

func TestDeleteStockEntryDraft(t *testing.T) {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token k:s" {
			t.Fatalf("auth header mismatch: %q", got)
		}
		if r.Method != http.MethodDelete || r.URL.Path != "/api/resource/Stock Entry/MAT-STE-2026-00001" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "k", "s")
	if err := c.DeleteStockEntryDraft(context.Background(), "MAT-STE-2026-00001"); err != nil {
		t.Fatalf("DeleteStockEntryDraft error: %v", err)
	}
}

func TestIsDuplicateBarcodeError(t *testing.T) {
	cases := []struct {
		errText string
		want    bool
	}{
		{"erp stock entry http 417: barcode must be unique", true},
		{"erp stock entry http 409: barcode already exists", true},
		{"erp stock entry http 500: duplicate entry 'ABC' for key 'barcode'", true},
		{"erp stock entry http 417: warehouse bo'sh", false},
	}

	for _, tc := range cases {
		got := IsDuplicateBarcodeError(errors.New(tc.errText))
		if got != tc.want {
			t.Fatalf("IsDuplicateBarcodeError(%q) = %v want %v", tc.errText, got, tc.want)
		}
	}
}
