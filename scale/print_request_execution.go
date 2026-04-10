package main

import (
	bridgestate "bridge/state"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type printRequestDecision string

const (
	printRequestNoop          printRequestDecision = "noop"
	printRequestMarkDone      printRequestDecision = "mark_done"
	printRequestExecute       printRequestDecision = "execute"
	printRequestErrorDisabled printRequestDecision = "error_disabled"
)

func decidePendingPrintRequest(req bridgestate.PrintRequestSnapshot, zebra ZebraStatus, activeEPC string, zebraEnabled bool) printRequestDecision {
	epc := strings.ToUpper(strings.TrimSpace(req.EPC))
	if epc == "" {
		return printRequestNoop
	}
	if strings.ToLower(strings.TrimSpace(req.Status)) != "pending" {
		return printRequestNoop
	}
	if strings.EqualFold(strings.TrimSpace(activeEPC), epc) {
		return printRequestNoop
	}
	if strings.EqualFold(strings.TrimSpace(zebra.LastEPC), epc) && strings.TrimSpace(zebra.Error) == "" {
		return printRequestMarkDone
	}
	if !zebraEnabled {
		return printRequestErrorDisabled
	}
	return printRequestExecute
}

func writePrintRequestStatus(store *bridgestate.Store, epc, status, errText string) error {
	if store == nil {
		return nil
	}

	epc = strings.ToUpper(strings.TrimSpace(epc))
	status = strings.ToLower(strings.TrimSpace(status))
	errText = strings.TrimSpace(errText)
	at := time.Now().UTC().Format(time.RFC3339Nano)

	return store.Update(func(snapshot *bridgestate.Snapshot) {
		if strings.ToUpper(strings.TrimSpace(snapshot.PrintRequest.EPC)) != epc {
			return
		}
		snapshot.PrintRequest.Status = status
		snapshot.PrintRequest.Error = errText
		snapshot.PrintRequest.UpdatedAt = at
	})
}

func (m *tuiModel) syncPendingPrintRequest(now time.Time) tea.Cmd {
	if m == nil || m.printRequest == nil {
		return nil
	}

	req, ok := m.printRequest.Pending(now)
	if !ok {
		return nil
	}

	lg := workerLog("worker.print_request")
	epc := strings.ToUpper(strings.TrimSpace(req.EPC))
	qtyText := formatQty(req.Qty, req.Unit)
	itemLabel := strings.TrimSpace(req.ItemName)
	if itemLabel == "" {
		itemLabel = strings.TrimSpace(req.ItemCode)
	}
	switch decidePendingPrintRequest(req, m.zebra, m.activePrintRequestEPC, m.zebraUpdates != nil) {
	case printRequestMarkDone:
		lg.Printf("request already satisfied: epc=%s item=%s qty=%s", epc, itemLabel, qtyText)
		if err := writePrintRequestStatus(m.bridgeStore, epc, "done", ""); err != nil {
			m.info = "print request status xato: " + err.Error()
			return nil
		}
		m.info = "print request already satisfied: epc=" + epc
		return nil
	case printRequestErrorDisabled:
		lg.Printf("request blocked: zebra disabled epc=%s item=%s qty=%s", epc, itemLabel, qtyText)
		if err := writePrintRequestStatus(m.bridgeStore, epc, "error", "zebra disabled"); err != nil {
			m.info = "print request status xato: " + err.Error()
			return nil
		}
		m.info = "print request xato: zebra disabled"
		return nil
	case printRequestExecute:
		lg.Printf("request queued: epc=%s item=%s qty=%s -> fake zebra encode", epc, itemLabel, qtyText)
		if err := writePrintRequestStatus(m.bridgeStore, epc, "processing", ""); err != nil {
			m.info = "print request status xato: " + err.Error()
			return nil
		}
		m.activePrintRequestEPC = epc
		m.info = "bridge print request queued: epc=" + epc
		return runEncodeEPCCmdWithEPC(m.zebraPreferred, epc, req.Qty, req.Unit, req.ItemName)
	default:
		return nil
	}
}

func formatQty(qty *float64, unit string) string {
	u := strings.TrimSpace(unit)
	if u == "" {
		u = "kg"
	}
	if qty == nil {
		return "- " + u
	}
	return fmt.Sprintf("%.3f %s", *qty, u)
}
