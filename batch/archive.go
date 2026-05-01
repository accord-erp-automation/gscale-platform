package batch

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

func EncodeArchiveBatchPayload(sessionID, itemName, qtyText, batchTime string) string {
	parts := []string{
		"ARCHIVE",
		strings.TrimSpace(sessionID),
		strings.TrimSpace(itemName),
		strings.TrimSpace(qtyText),
		strings.TrimSpace(batchTime),
	}
	raw := strings.Join(parts, "\n")
	return DefaultArchiveQRBaseURL + base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func BuildArchiveBatchLabel(input ArchiveBatchLabel, options LabelOptions) (ArchiveBatchData, error) {
	options = normalizeArchiveOptions(options)

	sessionID := SanitizeLabelText(input.SessionID)
	itemName := SanitizeLabelText(input.ItemName)
	qtyText := NormalizeKGValue(input.QtyText)
	batchTime := FormatArchiveBatchTime(input.BatchTime)
	if sessionID == "" {
		sessionID = "-"
	}
	if itemName == "" {
		itemName = "-"
	}
	if qtyText == "" {
		qtyText = "0"
	}
	if batchTime == "" {
		batchTime = "-"
	}

	qrPayload := EncodeArchiveBatchPayload(sessionID, itemName, qtyText, batchTime)
	labelWidthDots := MMDots(float64(options.LabelWidthMM), options.DPI)
	labelLengthDots := MMDots(float64(options.LabelLengthMM), options.DPI)
	safeMarginDots := MMDots(options.SafeMarginMM, options.DPI)
	leftX := safeMarginDots
	lineStep := MMDots(5.0, options.DPI)
	qrBoxDots := MMDots(options.QRBoxMM, options.DPI)
	qrRightGapDots := MMDots(2.0, options.DPI)
	qrX := maxInt(leftX, labelWidthDots-safeMarginDots-qrBoxDots-qrRightGapDots)
	qrY := safeMarginDots
	textWidthDots := maxInt(1, qrX-leftX-MMDots(2.0, options.DPI))
	itemLines := wrapTextForEZPL(itemName, textWidthDots, options.DPI, 1, 14, 8)
	if len(itemLines) == 0 {
		itemLines = []string{"-"}
	}

	itemY := safeMarginDots
	bruttoY := itemY + len(itemLines)*lineStep + lineStep
	nettoY := bruttoY + lineStep
	dateY := nettoY + lineStep
	maxDateY := maxInt(safeMarginDots+lineStep*4, labelLengthDots-safeMarginDots-MMDots(12.0, options.DPI))
	dateY = minInt(dateY, maxDateY)
	bruttoText := "BRUTTO: " + qtyText + " KG"
	nettoText := "NETTO: " + qtyText + " KG"
	dateText := "DATE: " + batchTime

	commands := []string{
		"~S,ESG",
		"^AD",
		"^XSET,IMMEDIATE,1",
		"^XSET,ACTIVERESPONSE,1",
		"^XSET,CODEPAGE,16",
		"^XSET,UNICODE,1",
		fmt.Sprintf("^Q%d,%d", options.LabelLengthMM, options.LabelGapMM),
		fmt.Sprintf("^W%d", options.LabelWidthMM),
		"^H10",
		"^P1",
		"^L",
	}
	for idx, line := range itemLines {
		y := itemY + idx*lineStep
		commands = append(commands, fmt.Sprintf("AC,%d,%d,1,1,0,0,%s", leftX, y, line))
	}
	commands = append(commands,
		fmt.Sprintf("AC,%d,%d,1,1,0,0,%s", leftX, bruttoY, bruttoText),
		fmt.Sprintf("AC,%d,%d,1,1,0,0,%s", leftX, nettoY, nettoText),
		fmt.Sprintf("AC,%d,%d,1,1,0,0,%s", leftX, dateY, dateText),
		fmt.Sprintf("W%d,%d,2,1,L,4,%d,%d,0", qrX, qrY, 4, len(qrPayload)),
		qrPayload,
		"E",
	)
	return ArchiveBatchData{
		Commands:  commands,
		QRPayload: qrPayload,
	}, nil
}

func normalizeArchiveOptions(options LabelOptions) LabelOptions {
	defaults := DefaultArchiveLabelOptions()
	if options.LabelLengthMM <= 0 {
		options.LabelLengthMM = defaults.LabelLengthMM
	}
	if options.LabelGapMM <= 0 {
		options.LabelGapMM = defaults.LabelGapMM
	}
	if options.LabelWidthMM <= 0 {
		options.LabelWidthMM = defaults.LabelWidthMM
	}
	if options.DPI <= 0 {
		options.DPI = defaults.DPI
	}
	if options.SafeMarginMM <= 0 {
		options.SafeMarginMM = defaults.SafeMarginMM
	}
	if options.QRBoxMM <= 0 {
		options.QRBoxMM = defaults.QRBoxMM
	}
	if options.RegularFont == "" {
		options.RegularFont = defaults.RegularFont
	}
	if options.BoldFont == "" {
		options.BoldFont = defaults.BoldFont
	}
	return options
}

func FormatArchiveBatchTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "-"
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, raw)
	}
	if err != nil {
		return raw
	}
	return parsed.Local().Format("02 Jan 2006 15:04")
}

func FormatArchiveBatchQty(qty float64) string {
	text := fmt.Sprintf("%.1f", roundToOneDecimal(qty))
	for strings.Contains(text, ".") && strings.HasSuffix(text, "0") {
		text = strings.TrimSuffix(text, "0")
	}
	return strings.TrimSuffix(text, ".")
}
