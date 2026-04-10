package erp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckConnection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token k:s" {
			t.Fatalf("auth header mismatch: %q", got)
		}
		if r.URL.Path != "/api/method/frappe.auth.get_logged_user" {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"Administrator"}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "k", "s")
	user, err := c.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection error: %v", err)
	}
	if user != "Administrator" {
		t.Fatalf("user mismatch: %q", user)
	}
}

func TestCheckConnection_UsesReadServiceWhenConfigured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected auth header: %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/healthz" {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewWithReadURL("https://erp.example.com", "k", "s", ts.URL)
	user, err := c.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection error: %v", err)
	}
	if user != "ERP DB Reader" {
		t.Fatalf("user mismatch: %q", user)
	}
}

func TestSearchItems(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token k:s" {
			t.Fatalf("auth header mismatch: %q", got)
		}
		if r.URL.Path != "/api/resource/Item" {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"name":"ITM-001","item_code":"ITM-001","item_name":"Apple"},{"name":"ITM-002","item_name":"Banana"}]}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "k", "s")
	items, err := c.SearchItems(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("SearchItems error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len mismatch: got=%d want=2", len(items))
	}
	if items[0].ItemCode != "ITM-001" || items[0].ItemName != "Apple" {
		t.Fatalf("item[0] mismatch: %+v", items[0])
	}
	if items[1].ItemCode != "ITM-002" || items[1].ItemName != "Banana" {
		t.Fatalf("item[1] mismatch: %+v", items[1])
	}
}

func TestSearchItems_UsesReadServiceWhenConfigured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected auth header: %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/items" {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "apple" {
			t.Fatalf("query mismatch: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"name":"ITM-001","item_code":"ITM-001","item_name":"Apple"}]}`))
	}))
	defer ts.Close()

	c := NewWithReadURL("https://erp.example.com", "k", "s", ts.URL)
	items, err := c.SearchItems(context.Background(), "apple", 10)
	if err != nil {
		t.Fatalf("SearchItems error: %v", err)
	}
	if len(items) != 1 || items[0].ItemCode != "ITM-001" {
		t.Fatalf("items mismatch: %+v", items)
	}
}

func TestSearchItemWarehouses(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token k:s" {
			t.Fatalf("auth header mismatch: %q", got)
		}
		if r.URL.Path != "/api/resource/Bin" {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("filters"); got == "" {
			t.Fatalf("filters is empty")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"warehouse":"Stores - A","actual_qty":12.5},{"warehouse":"Stores - B","actual_qty":7}]}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "k", "s")
	stocks, err := c.SearchItemWarehouses(context.Background(), "ITM-001", "", 10)
	if err != nil {
		t.Fatalf("SearchItemWarehouses error: %v", err)
	}
	if len(stocks) != 2 {
		t.Fatalf("stocks len mismatch: got=%d want=2", len(stocks))
	}
	if stocks[0].Warehouse != "Stores - A" || stocks[0].ActualQty != 12.5 {
		t.Fatalf("stocks[0] mismatch: %+v", stocks[0])
	}
	if stocks[1].Warehouse != "Stores - B" || stocks[1].ActualQty != 7 {
		t.Fatalf("stocks[1] mismatch: %+v", stocks[1])
	}
}

func TestSearchItemWarehouses_UsesReadServiceWhenConfigured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected auth header: %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/items/ITM-001/warehouses" {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"warehouse":"Stores - A","actual_qty":12.5}]}`))
	}))
	defer ts.Close()

	c := NewWithReadURL("https://erp.example.com", "k", "s", ts.URL)
	stocks, err := c.SearchItemWarehouses(context.Background(), "ITM-001", "", 10)
	if err != nil {
		t.Fatalf("SearchItemWarehouses error: %v", err)
	}
	if len(stocks) != 1 || stocks[0].Warehouse != "Stores - A" {
		t.Fatalf("stocks mismatch: %+v", stocks)
	}
}
