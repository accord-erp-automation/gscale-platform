package mobileapi

import (
	"strings"

	"core/runtimecfg"
)

type ERPSetup struct {
	ERPURL           string `json:"erp_url"`
	ERPReadURL       string `json:"erp_read_url"`
	ERPAPIKey        string `json:"erp_api_key"`
	ERPAPISecret     string `json:"erp_api_secret"`
	WarehouseMode    string `json:"warehouse_mode"`
	DefaultWarehouse string `json:"default_warehouse"`
}

func loadERPSetup(path string) (ERPSetup, error) {
	cfg, err := runtimecfg.Load(path)
	if err != nil {
		return ERPSetup{}, err
	}
	return ERPSetup{
		ERPURL:           strings.TrimSpace(cfg.ERPURL),
		ERPReadURL:       strings.TrimSpace(cfg.ERPReadURL),
		ERPAPIKey:        strings.TrimSpace(cfg.ERPAPIKey),
		ERPAPISecret:     strings.TrimSpace(cfg.ERPAPISecret),
		WarehouseMode:    normalizeWarehouseMode(cfg.WarehouseMode),
		DefaultWarehouse: strings.TrimSpace(cfg.DefaultWarehouse),
	}, nil
}

func saveERPSetup(path string, setup ERPSetup) error {
	current, _ := runtimecfg.Load(path)
	current.ERPURL = strings.TrimSpace(setup.ERPURL)
	current.ERPReadURL = strings.TrimSpace(setup.ERPReadURL)
	current.ERPAPIKey = strings.TrimSpace(setup.ERPAPIKey)
	current.ERPAPISecret = strings.TrimSpace(setup.ERPAPISecret)
	current.WarehouseMode = normalizeWarehouseMode(setup.WarehouseMode)
	current.DefaultWarehouse = strings.TrimSpace(setup.DefaultWarehouse)
	return runtimecfg.Save(path, current)
}

func clearERPSetup(path string) error {
	current, _ := runtimecfg.Load(path)
	current.ERPURL = ""
	current.ERPReadURL = ""
	current.ERPAPIKey = ""
	current.ERPAPISecret = ""
	return runtimecfg.Save(path, current)
}

func normalizeWarehouseMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "default") {
		return "default"
	}
	return "manual"
}
