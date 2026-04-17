package mobileapi

import "testing"

func TestSaveAndLoadERPSetup(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/mobile_setup.json"
	want := ERPSetup{
		ERPURL:           "http://localhost:8000",
		ERPReadURL:       "http://127.0.0.1:8090",
		ERPAPIKey:        "key-123",
		ERPAPISecret:     "secret-123",
		WarehouseMode:    "default",
		DefaultWarehouse: "Stores - A",
	}
	if err := saveERPSetup(path, want); err != nil {
		t.Fatalf("saveERPSetup: %v", err)
	}

	got, err := loadERPSetup(path)
	if err != nil {
		t.Fatalf("loadERPSetup: %v", err)
	}
	if got != want {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestLoadConfigUsesPersistedERPSetup(t *testing.T) {
	path := t.TempDir() + "/mobile_setup.json"
	if err := saveERPSetup(path, ERPSetup{
		ERPURL:           "http://localhost:8000",
		ERPReadURL:       "http://127.0.0.1:8090",
		ERPAPIKey:        "key-123",
		ERPAPISecret:     "secret-123",
		WarehouseMode:    "default",
		DefaultWarehouse: "Stores - A",
	}); err != nil {
		t.Fatalf("saveERPSetup: %v", err)
	}

	t.Setenv("MOBILE_API_SETUP_FILE", path)
	t.Setenv("ERP_URL", "")
	t.Setenv("ERP_API_KEY", "")
	t.Setenv("ERP_API_SECRET", "")

	cfg := LoadConfig()
	if cfg.ERPURL != "http://localhost:8000" {
		t.Fatalf("ERPURL = %q", cfg.ERPURL)
	}
	if cfg.ERPAPIKey != "key-123" {
		t.Fatalf("ERPAPIKey = %q", cfg.ERPAPIKey)
	}
	if cfg.ERPAPISecret != "secret-123" {
		t.Fatalf("ERPAPISecret = %q", cfg.ERPAPISecret)
	}
	if cfg.ERPReadURL != "http://127.0.0.1:8090" {
		t.Fatalf("ERPReadURL = %q", cfg.ERPReadURL)
	}
	if cfg.WarehouseMode != "default" {
		t.Fatalf("WarehouseMode = %q", cfg.WarehouseMode)
	}
	if cfg.DefaultWarehouse != "Stores - A" {
		t.Fatalf("DefaultWarehouse = %q", cfg.DefaultWarehouse)
	}
	if !cfg.HasERPWriteConfig() {
		t.Fatal("HasERPWriteConfig = false")
	}
}

func TestLoadConfigUsesPersistedWarehouseSetup(t *testing.T) {
	path := t.TempDir() + "/mobile_setup.json"
	if err := saveERPSetup(path, ERPSetup{
		WarehouseMode:    "default",
		DefaultWarehouse: "Stores - A",
	}); err != nil {
		t.Fatalf("saveERPSetup: %v", err)
	}

	t.Setenv("MOBILE_API_SETUP_FILE", path)
	t.Setenv("ERP_URL", "")
	t.Setenv("ERP_API_KEY", "")
	t.Setenv("ERP_API_SECRET", "")
	t.Setenv("WAREHOUSE_MODE", "")
	t.Setenv("DEFAULT_WAREHOUSE", "")

	cfg := LoadConfig()
	if cfg.WarehouseMode != "default" {
		t.Fatalf("WarehouseMode = %q", cfg.WarehouseMode)
	}
	if cfg.DefaultWarehouse != "Stores - A" {
		t.Fatalf("DefaultWarehouse = %q", cfg.DefaultWarehouse)
	}
	if !cfg.HasDefaultWarehouse() {
		t.Fatal("HasDefaultWarehouse = false")
	}
}

func TestLoadConfigWithoutERPWriteSetup(t *testing.T) {
	path := t.TempDir() + "/missing_setup.json"

	t.Setenv("MOBILE_API_SETUP_FILE", path)
	t.Setenv("ERP_URL", "")
	t.Setenv("ERP_API_KEY", "")
	t.Setenv("ERP_API_SECRET", "")
	t.Setenv("ERP_READ_URL", "")

	cfg := LoadConfig()
	if cfg.ListenAddr == "" {
		t.Fatal("ListenAddr should not be empty")
	}
	if cfg.ServerName == "" {
		t.Fatal("ServerName should not be empty")
	}
	if cfg.HasERPWriteConfig() {
		t.Fatal("HasERPWriteConfig should be false without env or persisted setup")
	}
	if cfg.WarehouseMode != "manual" {
		t.Fatalf("WarehouseMode = %q", cfg.WarehouseMode)
	}
}
