package runtimecfg

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const DefaultBridgeStateFile = "/tmp/gscale-zebra/bridge_state.json"

type Config struct {
	ERPURL          string
	ERPReadURL      string
	ERPAPIKey       string
	ERPAPISecret    string
	BridgeStateFile string
}

func Load(path string) (Config, error) {
	path = strings.TrimSpace(path)
	cfg := Config{
		ERPURL:          strings.TrimSpace(os.Getenv("ERP_URL")),
		ERPReadURL:      strings.TrimSpace(os.Getenv("ERP_READ_URL")),
		ERPAPIKey:       strings.TrimSpace(os.Getenv("ERP_API_KEY")),
		ERPAPISecret:    strings.TrimSpace(os.Getenv("ERP_API_SECRET")),
		BridgeStateFile: strings.TrimSpace(os.Getenv("BRIDGE_STATE_FILE")),
	}
	if path != "" {
		fileVals, err := parseEnvFile(path)
		if err != nil && !os.IsNotExist(err) {
			return Config{}, err
		}
		cfg = Config{
			ERPURL:          firstNonEmpty(cfg.ERPURL, fileVals["ERP_URL"], fileVals["URL"]),
			ERPReadURL:      firstNonEmpty(cfg.ERPReadURL, fileVals["ERP_READ_URL"]),
			ERPAPIKey:       firstNonEmpty(cfg.ERPAPIKey, fileVals["ERP_API_KEY"], fileVals["API_KEY"]),
			ERPAPISecret:    firstNonEmpty(cfg.ERPAPISecret, fileVals["ERP_API_SECRET"], fileVals["API_SECRET"]),
			BridgeStateFile: firstNonEmpty(cfg.BridgeStateFile, fileVals["BRIDGE_STATE_FILE"], DefaultBridgeStateFile),
		}
	}
	if cfg.BridgeStateFile == "" {
		cfg.BridgeStateFile = DefaultBridgeStateFile
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lines := []string{
		"ERP_URL=" + strings.TrimSpace(cfg.ERPURL),
		"ERP_READ_URL=" + strings.TrimSpace(cfg.ERPReadURL),
		"ERP_API_KEY=" + strings.TrimSpace(cfg.ERPAPIKey),
		"ERP_API_SECRET=" + strings.TrimSpace(cfg.ERPAPISecret),
		"BRIDGE_STATE_FILE=" + firstNonEmpty(strings.TrimSpace(cfg.BridgeStateFile), DefaultBridgeStateFile),
	}
	body := strings.Join(lines, "\n") + "\n"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Clear(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (c Config) HasERPWriteConfig() bool {
	return strings.TrimSpace(c.ERPURL) != "" &&
		strings.TrimSpace(c.ERPAPIKey) != "" &&
		strings.TrimSpace(c.ERPAPISecret) != ""
}

func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]string)
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := parseLine(line)
		if !ok {
			continue
		}
		out[normalizeKey(k)] = strings.TrimSpace(trimQuotes(v))
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseLine(line string) (string, string, bool) {
	if idx := strings.Index(line, "="); idx > 0 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
	}
	if idx := strings.Index(line, ":"); idx > 0 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
	}
	return "", "", false
}

func normalizeKey(k string) string {
	k = strings.TrimSpace(strings.ToUpper(k))
	repl := strings.NewReplacer(" ", "_", ".", "_", "-", "_")
	return repl.Replace(k)
}

func trimQuotes(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
