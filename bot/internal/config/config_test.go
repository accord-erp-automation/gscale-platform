package config

import (
	"os"
	"path/filepath"
	"testing"

	"core/runtimecfg"
)

func TestLoadSupportsColonAndAliasKeys(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, ".env")
	corePath := filepath.Join(d, "core.env")
	if err := runtimecfg.Save(corePath, runtimecfg.Config{
		ERPURL:          "https://erp.accord.uz",
		ERPReadURL:      "",
		ERPAPIKey:       "abc",
		ERPAPISecret:    "def",
		BridgeStateFile: runtimecfg.DefaultBridgeStateFile,
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORE_ENV_PATH", corePath)

	data := "telegram bot token:123:XYZ\n"
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.ERPURL != "https://erp.accord.uz" {
		t.Fatalf("ERPURL mismatch: %q", cfg.ERPURL)
	}
	if cfg.ERPAPIKey != "abc" {
		t.Fatalf("ERPAPIKey mismatch: %q", cfg.ERPAPIKey)
	}
	if cfg.ERPAPISecret != "def" {
		t.Fatalf("ERPAPISecret mismatch: %q", cfg.ERPAPISecret)
	}
	if cfg.TelegramBotToken != "123:XYZ" {
		t.Fatalf("TelegramBotToken mismatch: %q", cfg.TelegramBotToken)
	}
	if cfg.BridgeStateFile != runtimecfg.DefaultBridgeStateFile {
		t.Fatalf("BridgeStateFile mismatch: %q", cfg.BridgeStateFile)
	}
}

func TestLoadSupportsBridgeOverride(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, ".env")
	corePath := filepath.Join(d, "core.env")
	if err := runtimecfg.Save(corePath, runtimecfg.Config{
		ERPURL:          "https://erp.accord.uz",
		ERPReadURL:      "http://127.0.0.1:8090",
		ERPAPIKey:       "abc",
		ERPAPISecret:    "def",
		BridgeStateFile: "/tmp/custom-bridge.json",
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORE_ENV_PATH", corePath)

	data := "\n" +
		"TELEGRAM_BOT_TOKEN=123:XYZ\n"
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.BridgeStateFile != "/tmp/custom-bridge.json" {
		t.Fatalf("BridgeStateFile mismatch: %q", cfg.BridgeStateFile)
	}
	if cfg.ERPReadURL != "http://127.0.0.1:8090" {
		t.Fatalf("ERPReadURL mismatch: %q", cfg.ERPReadURL)
	}
}
