package main

import (
	bridgestate "bridge/state"
	"context"
	"fmt"
	"strings"
	"time"
)

type runtimeState struct {
	ctx                   context.Context
	updates               <-chan Reading
	zebraUpdates          <-chan ZebraStatus
	zebraPreferred        string
	bridgeStore           *bridgestate.Store
	batchState            *batchStateReader
	printRequest          *printRequestReader
	batchActive           bool
	message               string
	info                  string
	last                  Reading
	zebra                 ZebraStatus
	now                   time.Time
	activePrintRequestEPC string
	zebraEnabled          bool
}

func newRuntimeState(ctx context.Context, updates <-chan Reading, zebraUpdates <-chan ZebraStatus, zebraPreferred string, bridgeStateFile string, autoWhenNoBatch bool, serialErr error) *runtimeState {
	rs := &runtimeState{
		ctx:          ctx,
		updates:      updates,
		zebraUpdates: zebraUpdates,
		bridgeStore:  bridgestate.New(bridgeStateFile),
		batchState:   newBatchStateReader(bridgeStateFile, autoWhenNoBatch),
		printRequest: newPrintRequestReader(bridgeStateFile),
		batchActive:  true,
		last:         Reading{Unit: "kg"},
		message:      "scale oqimi kutilmoqda",
		info:         "ready",
		now:          time.Now(),
		zebra: ZebraStatus{
			Connected: false,
			Verify:    "-",
			ReadLine1: "-",
			ReadLine2: "-",
			UpdatedAt: time.Now(),
		},
		zebraEnabled: zebraUpdates != nil,
	}
	if rs.batchState != nil {
		rs.batchActive = rs.batchState.Active(time.Now())
	}
	if serialErr != nil {
		rs.message = serialErr.Error()
	}
	if !rs.zebraEnabled {
		rs.zebra.Error = "disabled"
	}
	return rs
}

func (rs *runtimeState) applyReading(upd Reading) {
	if rs == nil {
		return
	}
	if upd.Unit == "" && rs.last.Unit != "" {
		upd.Unit = rs.last.Unit
	}

	prevBatchActive := rs.batchActive
	if rs.batchState != nil {
		rs.batchActive = rs.batchState.Active(time.Now())
	}
	if prevBatchActive != rs.batchActive {
		if rs.batchActive {
			rs.info = "batch active: ERP workflow yoqildi"
		} else {
			rs.info = "batch inactive: ERP workflow to'xtadi"
		}
	}

	rs.last = upd
	if err := writeBridgeStateSnapshot(rs.bridgeStore, upd, rs.zebra); err != nil {
		rs.info = "bridge snapshot xato: " + err.Error()
	}
	if upd.Error != "" {
		rs.message = upd.Error
	} else {
		rs.message = "ok"
	}
}

func (rs *runtimeState) applyZebra(st ZebraStatus) {
	if rs == nil {
		return
	}
	st = mergeZebraStatus(rs.zebra, st)
	rs.zebra = st
	if err := writeBridgeStateSnapshot(rs.bridgeStore, rs.last, rs.zebra); err != nil {
		rs.info = "bridge snapshot xato: " + err.Error()
	}
	if rs.activePrintRequestEPC != "" && strings.EqualFold(strings.TrimSpace(st.Action), "encode") {
		status := "done"
		errText := ""
		if strings.TrimSpace(st.Error) != "" {
			status = "error"
			errText = st.Error
		}
		if err := writePrintRequestStatus(rs.bridgeStore, rs.activePrintRequestEPC, status, errText); err != nil {
			rs.info = "print request status xato: " + err.Error()
		}
		rs.activePrintRequestEPC = ""
	}
	if st.Action != "" {
		rs.info = zebraActionSummary(st)
	}
	if st.Error != "" && st.Action != "" {
		rs.info = zebraActionSummary(st)
	}
}

func (rs *runtimeState) processPendingPrintRequest(now time.Time) {
	if rs == nil || rs.printRequest == nil {
		return
	}

	req, ok := rs.printRequest.Pending(now)
	if !ok {
		return
	}

	lg := workerLog("worker.print_request")
	epc := strings.ToUpper(strings.TrimSpace(req.EPC))
	qtyText := formatQty(req.Qty, req.Unit)
	itemLabel := strings.TrimSpace(req.ItemName)
	if itemLabel == "" {
		itemLabel = strings.TrimSpace(req.ItemCode)
	}

	switch decidePendingPrintRequest(req, rs.zebra, rs.activePrintRequestEPC, rs.zebraEnabled, rs.last) {
	case printRequestMarkDone:
		lg.Printf("request already satisfied: epc=%s item=%s qty=%s", epc, itemLabel, qtyText)
		if err := writePrintRequestStatus(rs.bridgeStore, epc, "done", ""); err != nil {
			rs.info = "print request status xato: " + err.Error()
			return
		}
		rs.info = "print request already satisfied: epc=" + epc
	case printRequestErrorDisabled:
		lg.Printf("request blocked: zebra disabled epc=%s item=%s qty=%s", epc, itemLabel, qtyText)
		if err := writePrintRequestStatus(rs.bridgeStore, epc, "error", "zebra disabled"); err != nil {
			rs.info = "print request status xato: " + err.Error()
			return
		}
		rs.info = "print request xato: zebra disabled"
	case printRequestExternalExec:
		lg.Printf("request delegated: polygon fake zebra will handle epc=%s item=%s qty=%s", epc, itemLabel, qtyText)
		rs.info = "print request delegated to polygon: epc=" + epc
	case printRequestExecute:
		lg.Printf("request queued: epc=%s item=%s qty=%s -> fake zebra encode", epc, itemLabel, qtyText)
		if err := writePrintRequestStatus(rs.bridgeStore, epc, "processing", ""); err != nil {
			rs.info = "print request status xato: " + err.Error()
			return
		}
		rs.activePrintRequestEPC = epc
		rs.info = "bridge print request queued: epc=" + epc
		st := runZebraEncodeAndRead(rs.zebraPreferred, epc, formatLabelQty(req.Qty, req.Unit), req.ItemName, 1400*time.Millisecond)
		st.UpdatedAt = time.Now()
		rs.applyZebra(st)
	default:
		return
	}
}

func mergeZebraStatus(prev ZebraStatus, incoming ZebraStatus) ZebraStatus {
	st := incoming
	if strings.TrimSpace(st.LastEPC) == "" && strings.TrimSpace(prev.LastEPC) != "" {
		st.LastEPC = prev.LastEPC
		if strings.TrimSpace(st.Verify) == "" || strings.TrimSpace(st.Verify) == "-" {
			st.Verify = prev.Verify
		}
		// Monitor heartbeat eski EPC ni qayta vaqtlab yubormasin.
		if !prev.UpdatedAt.IsZero() {
			st.UpdatedAt = prev.UpdatedAt
		}
	}
	if st.UpdatedAt.IsZero() {
		st.UpdatedAt = time.Now()
	}
	return st
}

func zebraActionSummary(st ZebraStatus) string {
	a := strings.ToUpper(strings.TrimSpace(st.Action))
	if a == "" {
		a = "MONITOR"
	}
	if strings.TrimSpace(st.Error) != "" {
		return fmt.Sprintf("zebra %s xato: %s", strings.ToLower(a), st.Error)
	}
	if a == "ENCODE" {
		return fmt.Sprintf("zebra encode: epc=%s verify=%s line1=%s", safeText("-", st.LastEPC), safeText("UNKNOWN", st.Verify), safeText("-", st.ReadLine1))
	}
	if a == "READ" {
		return fmt.Sprintf("zebra read: verify=%s line1=%s", safeText("UNKNOWN", st.Verify), safeText("-", st.ReadLine1))
	}
	return fmt.Sprintf("zebra %s: ok", strings.ToLower(a))
}
