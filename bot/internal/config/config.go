package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const defaultBridgeStateFile = "/tmp/gscale-zebra/bridge_state.json"

type Config struct {
	TelegramBotToken string
	ERPURL           string
	ERPReadURL       string
	ERPAPIKey        string
	ERPAPISecret     string
	BridgeStateFile  string
}

func Load(envPath string) (Config, error) {
	if strings.TrimSpace(envPath) == "" {
		envPath = ".env"
	}

	fileVals, err := parseEnvFile(envPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}

	cfg := Config{
		TelegramBotToken: firstNonEmpty(
			os.Getenv("TELEGRAM_BOT_TOKEN"),
			fileVals["TELEGRAM_BOT_TOKEN"],
			fileVals["BOT_TOKEN"],
			fileVals["TOKEN"],
		),
		ERPURL: firstNonEmpty(
			os.Getenv("ERP_URL"),
			fileVals["ERP_URL"],
			fileVals["URL"],
		),
		ERPReadURL: firstNonEmpty(
			os.Getenv("ERP_READ_URL"),
			fileVals["ERP_READ_URL"],
		),
		ERPAPIKey: firstNonEmpty(
			os.Getenv("ERP_API_KEY"),
			fileVals["ERP_API_KEY"],
			fileVals["API_KEY"],
		),
		ERPAPISecret: firstNonEmpty(
			os.Getenv("ERP_API_SECRET"),
			fileVals["ERP_API_SECRET"],
			fileVals["API_SECRET"],
		),
		BridgeStateFile: firstNonEmpty(
			os.Getenv("BRIDGE_STATE_FILE"),
			fileVals["BRIDGE_STATE_FILE"],
			defaultBridgeStateFile,
		),
	}

	if err := cfg.Validate(); err != nil {
		abs, _ := filepath.Abs(envPath)
		return Config{}, fmt.Errorf("config invalid (%s): %w", abs, err)
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.TelegramBotToken) == "" {
		return errors.New("TELEGRAM_BOT_TOKEN bo'sh")
	}
	if strings.TrimSpace(c.ERPURL) == "" {
		return errors.New("ERP_URL bo'sh")
	}
	if strings.TrimSpace(c.ERPAPIKey) == "" {
		return errors.New("ERP_API_KEY bo'sh")
	}
	if strings.TrimSpace(c.ERPAPISecret) == "" {
		return errors.New("ERP_API_SECRET bo'sh")
	}
	if strings.TrimSpace(c.BridgeStateFile) == "" {
		return errors.New("BRIDGE_STATE_FILE bo'sh")
	}

	u, err := url.Parse(strings.TrimSpace(c.ERPURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("ERP_URL noto'g'ri (example: https://erp.accord.uz)")
	}
	if strings.TrimSpace(c.ERPReadURL) != "" {
		u, err = url.Parse(strings.TrimSpace(c.ERPReadURL))
		if err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("ERP_READ_URL noto'g'ri (example: http://127.0.0.1:8090)")
		}
	}
	return nil
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
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
