package mobileapi

import (
	"strings"

	"core/workflow"
)

func normalizePrintMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", workflow.PrintModeRFID, "rfid-label", "rfid_label", "rfidprint":
		return workflow.PrintModeRFID
	case workflow.PrintModeLabelOnly, "label-only", "label_only", "plain", "plain-label", "plain_label", "simple":
		return workflow.PrintModeLabelOnly
	default:
		return workflow.PrintModeRFID
	}
}
