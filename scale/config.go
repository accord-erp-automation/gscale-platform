package main

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const defaultSharedBridgeStateFile = "/tmp/gscale-zebra/bridge_state.json"

type appConfig struct {
	device          string
	bauds           []int
	unit            string
	probeTimeout    time.Duration
	bridgeURL       string
	bridgeInterval  time.Duration
	disableBridge   bool
	zebraDevice     string
	zebraInterval   time.Duration
	disableZebra    bool
	botDir          string
	disableBot      bool
	disableTUI      bool
	bridgeStateFile string
}

func parseFlags() (appConfig, error) {
	cfg := appConfig{}
	preferredBaud := 9600
	baudListRaw := "9600,19200,38400,57600,115200"

	flag.StringVar(&cfg.device, "device", "", "serial device path, example /dev/ttyUSB0")
	flag.IntVar(&preferredBaud, "baud", 9600, "preferred baudrate")
	flag.StringVar(&baudListRaw, "baud-list", "9600,19200,38400,57600,115200", "comma-separated baudrates for auto-detect")
	flag.StringVar(&cfg.unit, "unit", "kg", "default unit")
	flag.DurationVar(&cfg.probeTimeout, "probe-timeout", 800*time.Millisecond, "probe duration per port/baud")
	flag.StringVar(&cfg.bridgeURL, "bridge-url", "http://127.0.0.1:18000/api/v1/scale", "fallback HTTP endpoint")
	flag.DurationVar(&cfg.bridgeInterval, "bridge-interval", 120*time.Millisecond, "bridge poll interval")
	flag.BoolVar(&cfg.disableBridge, "no-bridge", false, "disable HTTP bridge fallback")
	flag.StringVar(&cfg.zebraDevice, "zebra-device", "", "zebra printer path, example /dev/usb/lp0")
	flag.DurationVar(&cfg.zebraInterval, "zebra-interval", 900*time.Millisecond, "zebra monitor poll interval")
	flag.BoolVar(&cfg.disableZebra, "no-zebra", false, "disable zebra monitor/actions in TUI")
	flag.StringVar(&cfg.botDir, "bot-dir", "../bot", "telegram bot module directory")
	flag.BoolVar(&cfg.disableBot, "no-bot", false, "disable auto-start telegram bot")
	flag.BoolVar(&cfg.disableTUI, "no-tui", false, "disable the Bubble Tea UI and run headless")
	flag.StringVar(&cfg.bridgeStateFile, "bridge-state-file", defaultSharedBridgeStateFile, "shared bridge JSON file for scale+zebra+bot")
	flag.Parse()

	bauds, err := parseBaudList(baudListRaw, preferredBaud)
	if err != nil {
		return appConfig{}, err
	}
	cfg.bauds = bauds

	return cfg, nil
}

func parseBaudList(raw string, preferred int) ([]int, error) {
	seen := map[int]bool{}
	out := make([]int, 0, 8)
	add := func(b int) {
		if b <= 0 || seen[b] {
			return
		}
		seen[b] = true
		out = append(out, b)
	}

	add(preferred)
	for _, part := range strings.Split(raw, ",") {
		v := strings.TrimSpace(part)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid baudrate %q", v)
		}
		add(n)
	}

	if len(out) == 0 {
		return nil, errors.New("empty baud list")
	}

	return out, nil
}
