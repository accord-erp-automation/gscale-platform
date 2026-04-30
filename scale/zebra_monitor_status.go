package main

import (
	"context"
	"time"
)

func startZebraMonitor(ctx context.Context, preferredDevice string, interval time.Duration, out chan<- ZebraStatus) {
	if out == nil {
		return
	}
	if interval < 300*time.Millisecond {
		interval = 300 * time.Millisecond
	}
	lg := workerLog("worker.zebra_monitor")
	lg.Printf("start: preferred_device=%s interval=%s", preferredDevice, interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		waiting := false

		st := collectZebraStatus(preferredDevice, 900*time.Millisecond)
		publishZebraStatus(out, st)
		if st.Connected {
			lg.Printf("ready: zebra device=%s verify=%s", st.DevicePath, st.Verify)
		} else {
			lg.Printf("wait: zebra device=%s", preferredDevice)
			waiting = true
		}
		for {
			select {
			case <-ctx.Done():
				lg.Printf("stop: context done")
				return
			case <-ticker.C:
				st := collectZebraStatus(preferredDevice, 900*time.Millisecond)
				publishZebraStatus(out, st)
				switch {
				case st.Connected && waiting:
					lg.Printf("ready: zebra device=%s verify=%s", st.DevicePath, st.Verify)
					waiting = false
				case !st.Connected && !waiting:
					lg.Printf("wait: zebra device=%s", preferredDevice)
					waiting = true
				}
			}
		}
	}()
}

func collectZebraStatus(preferredDevice string, _ time.Duration) ZebraStatus {
	st := ZebraStatus{
		Connected: false,
		Verify:    "-",
		UpdatedAt: time.Now(),
	}

	p, err := SelectZebraPrinter(preferredDevice)
	if err != nil {
		st.Error = err.Error()
		return st
	}

	st.Connected = true
	st.DevicePath = p.DevicePath
	st.Name = p.DisplayName()
	st.DeviceState = "-"
	st.MediaState = "-"
	st.ReadLine1 = "-"
	st.ReadLine2 = "-"
	st.Verify = "-"
	return st
}

func applyZebraSnapshot(st *ZebraStatus, p ZebraPrinter, timeout time.Duration) {
	st.DeviceState = safeText("-", queryVarRetry(p.DevicePath, "device.status", timeout, 3, 90*time.Millisecond))
	st.MediaState = safeText("-", queryVarRetry(p.DevicePath, "media.status", timeout, 3, 90*time.Millisecond))
	st.ReadLine1 = "-"
	st.ReadLine2 = "-"
}

func publishZebraStatus(ch chan<- ZebraStatus, st ZebraStatus) {
	select {
	case ch <- st:
	default:
	}
}
