package main

import (
	"context"
	"strings"
	"time"
)

func runHeadless(ctx context.Context, updates <-chan Reading, zebraUpdates <-chan ZebraStatus, sourceLine string, zebraPreferred string, bridgeStateFile string, autoWhenNoBatch bool, serialErr error) error {
	m := newRuntimeModel(ctx, updates, zebraUpdates, sourceLine, zebraPreferred, bridgeStateFile, autoWhenNoBatch, serialErr)
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
			m.applyReading(upd)
		case st, ok := <-zebraCh:
			if !ok {
				zebraCh = nil
				continue
			}
			m.applyZebra(st)
		case now := <-ticker.C:
			if reqCmd := m.syncPendingPrintRequest(now); reqCmd != nil {
				if msg := reqCmd(); msg != nil {
					if st, ok := msg.(zebraMsg); ok {
						m.applyZebra(st.status)
					}
				}
			}
		}
	}
}

func (m *tuiModel) applyReading(upd Reading) {
	if m == nil {
		return
	}
	if upd.Unit == "" && m.last.Unit != "" {
		upd.Unit = m.last.Unit
	}

	prevBatchActive := m.batchActive
	if m.batchState != nil {
		m.batchActive = m.batchState.Active(time.Now())
	}
	if prevBatchActive != m.batchActive {
		if m.batchActive {
			m.info = "batch active: ERP workflow yoqildi"
		} else {
			m.info = "batch inactive: ERP workflow to'xtadi"
		}
	}

	m.last = upd
	if err := writeBridgeStateSnapshot(m.bridgeStore, upd, m.zebra); err != nil {
		m.info = "bridge snapshot xato: " + err.Error()
	}
	if upd.Error != "" {
		m.message = upd.Error
	} else {
		m.message = "ok"
	}
}

func (m *tuiModel) applyZebra(st ZebraStatus) {
	if m == nil {
		return
	}
	st = mergeZebraStatus(m.zebra, st)
	m.zebra = st
	if err := writeBridgeStateSnapshot(m.bridgeStore, m.last, m.zebra); err != nil {
		m.info = "bridge snapshot xato: " + err.Error()
	}
	if m.activePrintRequestEPC != "" && strings.EqualFold(strings.TrimSpace(st.Action), "encode") {
		status := "done"
		errText := ""
		if strings.TrimSpace(st.Error) != "" {
			status = "error"
			errText = st.Error
		}
		if err := writePrintRequestStatus(m.bridgeStore, m.activePrintRequestEPC, status, errText); err != nil {
			m.info = "print request status xato: " + err.Error()
		}
		m.activePrintRequestEPC = ""
	}
	if st.Action != "" {
		m.info = zebraActionSummary(st)
	}
	if st.Error != "" && st.Action != "" {
		m.info = zebraActionSummary(st)
	}
}
