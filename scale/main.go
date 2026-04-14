package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	cfg, err := parseFlags()
	if err != nil {
		exitErr(err)
	}

	if err := initWorkflowLogs(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: workflow logs init xato: %v\n", err)
	}
	defer closeWorkflowLogs()
	workerLog("main").Printf("scale started")
	if scaleWorkflowLogs != nil {
		workerLog("main").Printf("workflow logs dir: %s", scaleWorkflowLogs.Dir())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	updates := make(chan Reading, 32)
	var zebraUpdates <-chan ZebraStatus
	var sourceLine string
	var serialErr error
	started := false

	port, usedBaud, err := detectScalePort(cfg.device, cfg.bauds, cfg.probeTimeout, cfg.unit)
	if err == nil {
		if startErr := startSerialReader(ctx, port, usedBaud, cfg.unit, updates); startErr == nil {
			workerLog("main").Printf("serial reader started: device=%s baud=%d", port, usedBaud)
			sourceLine = fmt.Sprintf("serial (%s @ %d)", port, usedBaud)
			started = true
		} else {
			workerLog("main").Printf("serial reader start error: %v", startErr)
			serialErr = startErr
		}
	} else {
		workerLog("main").Printf("serial detect error: %v", err)
		serialErr = err
	}

	if !started && !cfg.disableBridge && strings.TrimSpace(cfg.bridgeURL) != "" {
		startBridgeReader(ctx, strings.TrimSpace(cfg.bridgeURL), cfg.bridgeInterval, updates)
		workerLog("main").Printf("bridge reader started: url=%s", strings.TrimSpace(cfg.bridgeURL))
		sourceLine = fmt.Sprintf("bridge (%s)", strings.TrimSpace(cfg.bridgeURL))
		started = true
	}

	if !started {
		if serialErr != nil {
			exitErr(serialErr)
		}
		exitErr(errors.New("scale source not available"))
	}

	if !cfg.disableZebra {
		zch := make(chan ZebraStatus, 16)
		startZebraMonitor(ctx, cfg.zebraDevice, cfg.zebraInterval, zch)
		workerLog("main").Printf("zebra monitor started: device=%s interval=%s", cfg.zebraDevice, cfg.zebraInterval)
		zebraUpdates = zch
	}

	var botProc *BotProcess
	if !cfg.disableBot {
		bp, err := startBotProcess(cfg.botDir)
		if err != nil {
			workerLog("main").Printf("bot auto-start warning: %v", err)
			fmt.Fprintf(os.Stderr, "warning: bot auto-start bo'lmadi: %v\n", err)
		} else {
			botProc = bp
			defer func() {
				if stopErr := botProc.Stop(3 * time.Second); stopErr != nil {
					workerLog("main").Printf("bot stop warning: %v", stopErr)
					fmt.Fprintf(os.Stderr, "warning: bot stop xato: %v\n", stopErr)
				}
			}()
		}
	}

	runFn := runTUI
	runName := "tui"
	if cfg.disableTUI {
		runFn = runHeadless
		runName = "headless"
	}
	if err := runFn(ctx, updates, zebraUpdates, sourceLine, cfg.zebraDevice, cfg.bridgeStateFile, cfg.disableBot, serialErr); err != nil {
		workerLog("main").Printf("%s run error: %v", runName, err)
		cancel()
		if botProc != nil {
			if stopErr := botProc.Stop(3 * time.Second); stopErr != nil {
				workerLog("main").Printf("bot stop warning: %v", stopErr)
				fmt.Fprintf(os.Stderr, "warning: bot stop xato: %v\n", stopErr)
			}
		}
		exitErr(err)
	}
}
