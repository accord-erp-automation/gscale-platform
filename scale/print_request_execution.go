package main

import (
	bridgestate "bridge/state"
	"fmt"
	"math"
	"strconv"
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

const (
	printRequestModeRFID      = "rfid"
	printRequestModeLabelOnly = "label"
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
	if normalizePrintRequestMode(req.Mode) != printRequestModeLabelOnly &&
		strings.EqualFold(strings.TrimSpace(zebra.LastEPC), epc) &&
		strings.TrimSpace(zebra.Error) == "" {
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

func normalizePrintRequestMode(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "", printRequestModeRFID, "rfid-label", "rfid_label", "rfidprint":
		return printRequestModeRFID
	case printRequestModeLabelOnly, "label-only", "label_only", "plain", "plain-label", "plain_label", "simple":
		return printRequestModeLabelOnly
	default:
		return printRequestModeRFID
	}
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
	return fmt.Sprintf("%s %s", formatRoundedQty(*qty), u)
}

func formatGoDEXQty(qty *float64, unit string) string {
	if qty == nil {
		return "-"
	}
	return formatRoundedQty(*qty)
}

type printWeightLabels struct {
	Netto   string
	Brutto  string
	HasTare bool
}

func formatPrintWeightLabels(req bridgestate.PrintRequestSnapshot) printWeightLabels {
	netQty := req.Qty
	grossQty := req.GrossQty
	if grossQty == nil {
		grossQty = req.Qty
	}
	if !req.Tare || req.TareKG <= 0 || grossQty == nil {
		return printWeightLabels{Netto: formatLabelQty(req.Qty, req.Unit)}
	}
	if netQty == nil {
		net := *grossQty - req.TareKG
		if net < 0 {
			net = 0
		}
		netQty = &net
	}
	return printWeightLabels{
		Netto:   formatTrimmedQty(*netQty, req.Unit),
		Brutto:  formatTrimmedQty(*grossQty, req.Unit),
		HasTare: true,
	}
}

func formatGoDEXWeightLabels(req bridgestate.PrintRequestSnapshot, defaultBrutto string) printWeightLabels {
	labels := formatPrintWeightLabels(req)
	if labels.HasTare {
		return printWeightLabels{
			Netto:   stripKGUnit(labels.Netto),
			Brutto:  stripKGUnit(labels.Brutto),
			HasTare: true,
		}
	}
	return printWeightLabels{
		Netto:  formatGoDEXQty(req.Qty, req.Unit),
		Brutto: strings.TrimSpace(defaultBrutto),
	}
}

func formatTrimmedQty(qty float64, unit string) string {
	u := strings.TrimSpace(unit)
	if u == "" {
		u = "kg"
	}
	return formatRoundedQty(qty) + " " + u
}

func formatRoundedQty(qty float64) string {
	rounded := math.Round(qty*10) / 10
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

func stripKGUnit(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, "kg")
	v = strings.TrimSuffix(v, "KG")
	return strings.TrimSpace(v)
}
