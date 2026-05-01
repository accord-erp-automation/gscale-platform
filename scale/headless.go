package main

import (
	"context"
	"strings"
	"time"
)

func runHeadless(ctx context.Context, updates <-chan Reading, zebraUpdates <-chan ZebraStatus, serialDevice string, serialBaud int, zebraPreferred string, bridgeStateFile string, autoWhenNoBatch bool, serialErr error, status *consoleStatus, printBackend string, godexCompany string, godexBrutto string) error {
	rs := newRuntimeState(ctx, updates, zebraUpdates, zebraPreferred, bridgeStateFile, autoWhenNoBatch, serialErr, printBackend, godexCompany, godexBrutto)
	lg := workerLog("main")
	lg.Printf("headless mode started")
	if status != nil {
		status.SetSerial(waitSerialLine(serialDevice, serialBaud))
		status.SetZebra(waitZebraLine(zebraPreferred))
		status.Render()
	}

	rs.refreshPrinterSnapshot(time.Now())

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
			if status != nil {
				status.SetSerial(serialStatusLine(upd, serialDevice, serialBaud))
			}
		case st, ok := <-zebraCh:
			if !ok {
				zebraCh = nil
				continue
			}
			rs.applyZebra(st)
			if status != nil {
				status.SetZebra(zebraStatusLine(st, zebraPreferred))
			}
		case now := <-ticker.C:
			rs.refreshPrinterSnapshot(now)
			rs.processPendingPrintRequest(now)
		}
	}
}

func serialStatusLine(upd Reading, device string, baud int) string {
	device = strings.TrimSpace(device)
	switch strings.ToLower(strings.TrimSpace(upd.Source)) {
	case "bridge":
		return "connected: bridge"
	case "serial":
		if strings.TrimSpace(upd.Error) != "" {
			return waitSerialLine(device, baud)
		}
		return readySerialLine(device, baud)
	default:
		if strings.TrimSpace(upd.Error) != "" {
			return waitSerialLine(device, baud)
		}
		if strings.TrimSpace(upd.Source) != "" || upd.Weight != nil || strings.TrimSpace(upd.Raw) != "" {
			return readySerialLine(device, baud)
		}
		return waitSerialLine(device, baud)
	}
}

func zebraStatusLine(st ZebraStatus, preferred string) string {
	device := strings.TrimSpace(st.DevicePath)
	if device == "" {
		device = strings.TrimSpace(preferred)
	}
	if st.Connected {
		return readyZebraLine(device)
	}
	return waitZebraLine(device)
}
