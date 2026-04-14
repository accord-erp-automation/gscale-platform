package main

import (
	"context"
	"time"
)

func runHeadless(ctx context.Context, updates <-chan Reading, zebraUpdates <-chan ZebraStatus, zebraPreferred string, bridgeStateFile string, autoWhenNoBatch bool, serialErr error) error {
	rs := newRuntimeState(ctx, updates, zebraUpdates, zebraPreferred, bridgeStateFile, autoWhenNoBatch, serialErr)
	lg := workerLog("main")
	lg.Printf("headless mode started")

	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()

	var zebraCh <-chan ZebraStatus = zebraUpdates
	for {
		select {
		case <-ctx.Done():
			lg.Printf("headless mode stopped: context done")
			return nil
		case upd, ok := <-updates:
			if !ok {
				lg.Printf("headless mode stopped: reading channel closed")
				return nil
			}
			rs.applyReading(upd)
		case st, ok := <-zebraCh:
			if !ok {
				zebraCh = nil
				continue
			}
			rs.applyZebra(st)
		case now := <-ticker.C:
			rs.processPendingPrintRequest(now)
		}
	}
}
