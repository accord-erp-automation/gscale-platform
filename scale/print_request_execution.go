package main

import (
	bridgestate "bridge/state"
	"fmt"
	"strings"
	"time"
)

type printRequestDecision string

const (
	printRequestNoop          printRequestDecision = "noop"
	printRequestMarkDone      printRequestDecision = "mark_done"
	printRequestExecute       printRequestDecision = "execute"
	printRequestErrorDisabled printRequestDecision = "error_disabled"
	printRequestExternalExec  printRequestDecision = "external_exec"
)

func decidePendingPrintRequest(req bridgestate.PrintRequestSnapshot, zebra ZebraStatus, activeEPC string, zebraEnabled bool, rd Reading) printRequestDecision {
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
		if strings.HasPrefix(strings.TrimSpace(rd.Port), "polygon://") {
			return printRequestExternalExec
		}
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

func formatQty(qty *float64, unit string) string {
	return formatLabelQty(qty, unit)
}

func formatLabelQty(qty *float64, unit string) string {
	u := strings.TrimSpace(unit)
	if u == "" {
		u = "kg"
	}
	if qty == nil {
		return "- " + u
	}
	return fmt.Sprintf("%.3f %s", *qty, u)
}
